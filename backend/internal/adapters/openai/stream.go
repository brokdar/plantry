package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// parseStream consumes OpenAI SSE chunks until `data: [DONE]`, relays canonical
// events on out, and returns the assembled assistant message.
func parseStream(ctx context.Context, body io.Reader, out chan<- llm.Event) (*llm.Response, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var (
		dataBuf             bytes.Buffer
		textBuilder         strings.Builder
		toolCalls           = map[int]*toolCallAccum{} // keyed by index
		toolCallOrder       []int
		finishReason        string
		usage               llm.Usage
		model               string
		messageStartEmitted bool
	)

	emitMessageStart := func(m string) error {
		if messageStartEmitted {
			return nil
		}
		messageStartEmitted = true
		return send(ctx, out, llm.Event{Type: llm.EventMessageStart, Payload: llm.MessageStartPayload{Model: m}})
	}

	flush := func() error {
		defer dataBuf.Reset()
		if dataBuf.Len() == 0 {
			return nil
		}
		line := strings.TrimSpace(dataBuf.String())
		if line == "[DONE]" {
			return io.EOF
		}
		return handleChunk(ctx, []byte(line), out, &textBuilder, toolCalls, &toolCallOrder, &finishReason, &usage, &model, emitMessageStart)
	}

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				if err == io.EOF {
					break
				}
				return nil, err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(strings.TrimPrefix(line, "data: "))
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("openai: stream scan: %w", err)
	}
	if err := flush(); err != nil && err != io.EOF {
		return nil, err
	}

	// Assemble response.
	var content []llm.ContentBlock
	if textBuilder.Len() > 0 {
		content = append(content, llm.ContentBlock{Type: llm.ContentTypeText, Text: textBuilder.String()})
	}
	// Sort tool calls by their index order.
	sort.Ints(toolCallOrder)
	for _, idx := range toolCallOrder {
		tc := toolCalls[idx]
		if tc == nil {
			continue
		}
		args := strings.TrimSpace(tc.args.String())
		if args == "" {
			args = "{}"
		}
		var raw json.RawMessage
		if json.Valid([]byte(args)) {
			raw = json.RawMessage(args)
		} else {
			raw = json.RawMessage(fmt.Sprintf("%q", args))
		}
		content = append(content, llm.ContentBlock{
			Type: llm.ContentTypeToolUse, ToolUseID: tc.id, ToolUseName: tc.name, ToolUseInput: raw,
		})
	}

	return &llm.Response{
		Message:    llm.Message{Role: llm.RoleAssistant, Content: content},
		StopReason: mapFinishReason(finishReason),
		Usage:      usage,
	}, nil
}

type toolCallAccum struct {
	id   string
	name string
	args strings.Builder
	// toolCallStartEmitted marks whether we've already emitted ToolCallStart.
	toolCallStartEmitted bool
}

type chunkPayload struct {
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		TotalTokens         int `json:"total_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

func handleChunk(
	ctx context.Context,
	data []byte,
	out chan<- llm.Event,
	textBuilder *strings.Builder,
	toolCalls map[int]*toolCallAccum,
	toolCallOrder *[]int,
	finishReason *string,
	usage *llm.Usage,
	model *string,
	emitMessageStart func(string) error,
) error {
	var chunk chunkPayload
	if err := json.Unmarshal(data, &chunk); err != nil {
		return fmt.Errorf("openai: chunk decode: %w", err)
	}
	if chunk.Model != "" && *model == "" {
		*model = chunk.Model
		if err := emitMessageStart(chunk.Model); err != nil {
			return err
		}
	}
	if len(chunk.Choices) == 0 {
		// Usage-only final chunk (OpenAI with include_usage).
		if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
			usage.InputTokens = chunk.Usage.PromptTokens
			usage.OutputTokens = chunk.Usage.CompletionTokens
			usage.CacheReadTokens = chunk.Usage.PromptTokensDetails.CachedTokens
		}
		return nil
	}
	if err := emitMessageStart(*model); err != nil {
		return err
	}

	choice := chunk.Choices[0]
	if choice.Delta.Content != "" {
		textBuilder.WriteString(choice.Delta.Content)
		if err := send(ctx, out, llm.Event{Type: llm.EventAssistantDelta, Payload: llm.AssistantDeltaPayload{
			Text: choice.Delta.Content,
		}}); err != nil {
			return err
		}
	}
	for _, tc := range choice.Delta.ToolCalls {
		acc, ok := toolCalls[tc.Index]
		if !ok {
			acc = &toolCallAccum{}
			toolCalls[tc.Index] = acc
			*toolCallOrder = append(*toolCallOrder, tc.Index)
		}
		if tc.ID != "" {
			acc.id = tc.ID
		}
		if tc.Function.Name != "" {
			acc.name = tc.Function.Name
		}
		// First fragment with both id+name → emit tool_call_start.
		if !acc.toolCallStartEmitted && acc.id != "" && acc.name != "" {
			acc.toolCallStartEmitted = true
			if err := send(ctx, out, llm.Event{Type: llm.EventToolCallStart, Payload: llm.ToolCallStartPayload{
				ID: acc.id, Name: acc.name,
			}}); err != nil {
				return err
			}
		}
		if tc.Function.Arguments != "" {
			acc.args.WriteString(tc.Function.Arguments)
			if acc.id != "" {
				if err := send(ctx, out, llm.Event{Type: llm.EventToolCallDelta, Payload: llm.ToolCallDeltaPayload{
					ID: acc.id, ArgsJSON: tc.Function.Arguments,
				}}); err != nil {
					return err
				}
			}
		}
	}
	if choice.FinishReason != "" {
		*finishReason = choice.FinishReason
	}
	if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
		usage.InputTokens = chunk.Usage.PromptTokens
		usage.OutputTokens = chunk.Usage.CompletionTokens
		usage.CacheReadTokens = chunk.Usage.PromptTokensDetails.CachedTokens
	}
	return nil
}

func mapFinishReason(s string) llm.StopReason {
	switch s {
	case "stop":
		return llm.StopReasonEndTurn
	case "tool_calls", "function_call":
		return llm.StopReasonToolUse
	case "length":
		return llm.StopReasonMaxTokens
	case "content_filter":
		return llm.StopReasonRefusal
	case "":
		return llm.StopReasonEndTurn
	default:
		return llm.StopReason(s)
	}
}

func send(ctx context.Context, out chan<- llm.Event, evt llm.Event) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case out <- evt:
		return nil
	}
}
