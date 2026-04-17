package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// parseStream reads the Anthropic SSE event sequence from body, relays
// canonical llm.Events on out, and returns the assembled Response.
// body is drained but not closed here — the caller owns it.
func parseStream(ctx context.Context, body io.Reader, out chan<- llm.Event) (*llm.Response, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var (
		eventName string
		dataBuf   bytes.Buffer
	)

	// Per-stream accumulators.
	blocks := map[int]*blockAccum{}
	blockOrder := []int{}
	var stopReason llm.StopReason
	var usage llm.Usage
	var model string

	flushEvent := func() error {
		defer func() {
			eventName = ""
			dataBuf.Reset()
		}()
		if dataBuf.Len() == 0 {
			return nil
		}
		data := bytes.TrimRight(dataBuf.Bytes(), "\n")
		return handleSSEEvent(ctx, eventName, data, out, blocks, &blockOrder, &stopReason, &usage, &model)
	}

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		line := scanner.Text()
		if line == "" {
			if err := flushEvent(); err != nil {
				return nil, err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue // comment/heartbeat
		}
		if strings.HasPrefix(line, "event: ") {
			eventName = strings.TrimPrefix(line, "event: ")
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
		return nil, fmt.Errorf("anthropic: stream scan: %w", err)
	}
	if err := flushEvent(); err != nil {
		return nil, err
	}

	// Assemble the Response.
	content := make([]llm.ContentBlock, 0, len(blockOrder))
	for _, idx := range blockOrder {
		b, ok := blocks[idx]
		if !ok {
			continue
		}
		content = append(content, b.finalize())
	}
	return &llm.Response{
		Message:    llm.Message{Role: llm.RoleAssistant, Content: content},
		StopReason: stopReason,
		Usage:      usage,
	}, nil
}

type blockAccum struct {
	kind  string // "text" | "tool_use" | "thinking"
	text  strings.Builder
	id    string
	name  string
	args  strings.Builder
	input json.RawMessage
}

func (b *blockAccum) finalize() llm.ContentBlock {
	switch b.kind {
	case "tool_use":
		raw := b.input
		if len(raw) == 0 {
			s := strings.TrimSpace(b.args.String())
			if s == "" {
				s = "{}"
			}
			raw = json.RawMessage(s)
		}
		return llm.ContentBlock{
			Type: llm.ContentTypeToolUse, ToolUseID: b.id, ToolUseName: b.name, ToolUseInput: raw,
		}
	default:
		return llm.ContentBlock{Type: llm.ContentTypeText, Text: b.text.String()}
	}
}

func handleSSEEvent(
	ctx context.Context,
	name string,
	data []byte,
	out chan<- llm.Event,
	blocks map[int]*blockAccum,
	order *[]int,
	stopReason *llm.StopReason,
	usage *llm.Usage,
	model *string,
) error {
	switch name {
	case "message_start":
		var payload struct {
			Message struct {
				Model string `json:"model"`
				Usage struct {
					InputTokens              int `json:"input_tokens"`
					OutputTokens             int `json:"output_tokens"`
					CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
					CacheReadInputTokens     int `json:"cache_read_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("anthropic: message_start decode: %w", err)
		}
		*model = payload.Message.Model
		usage.InputTokens = payload.Message.Usage.InputTokens
		usage.OutputTokens = payload.Message.Usage.OutputTokens
		usage.CacheReadTokens = payload.Message.Usage.CacheReadInputTokens
		usage.CacheWriteTokens = payload.Message.Usage.CacheCreationInputTokens
		return send(ctx, out, llm.Event{Type: llm.EventMessageStart, Payload: llm.MessageStartPayload{Model: *model}})

	case "content_block_start":
		var payload struct {
			Index        int `json:"index"`
			ContentBlock struct {
				Type  string          `json:"type"`
				Text  string          `json:"text"`
				ID    string          `json:"id"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			} `json:"content_block"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("anthropic: content_block_start decode: %w", err)
		}
		blk := &blockAccum{kind: payload.ContentBlock.Type}
		switch payload.ContentBlock.Type {
		case "text":
			blk.text.WriteString(payload.ContentBlock.Text)
		case "tool_use":
			blk.id = payload.ContentBlock.ID
			blk.name = payload.ContentBlock.Name
			if len(payload.ContentBlock.Input) > 0 && string(payload.ContentBlock.Input) != "null" {
				blk.input = payload.ContentBlock.Input
			}
			if err := send(ctx, out, llm.Event{Type: llm.EventToolCallStart, Payload: llm.ToolCallStartPayload{
				ID: blk.id, Name: blk.name,
			}}); err != nil {
				return err
			}
		}
		blocks[payload.Index] = blk
		*order = append(*order, payload.Index)
		return nil

	case "content_block_delta":
		var payload struct {
			Index int `json:"index"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("anthropic: content_block_delta decode: %w", err)
		}
		blk := blocks[payload.Index]
		if blk == nil {
			return nil
		}
		switch payload.Delta.Type {
		case "text_delta":
			blk.text.WriteString(payload.Delta.Text)
			return send(ctx, out, llm.Event{Type: llm.EventAssistantDelta, Payload: llm.AssistantDeltaPayload{
				Text: payload.Delta.Text,
			}})
		case "input_json_delta":
			blk.args.WriteString(payload.Delta.PartialJSON)
			return send(ctx, out, llm.Event{Type: llm.EventToolCallDelta, Payload: llm.ToolCallDeltaPayload{
				ID: blk.id, ArgsJSON: payload.Delta.PartialJSON,
			}})
		}
		return nil

	case "content_block_stop":
		var payload struct {
			Index int `json:"index"`
		}
		_ = json.Unmarshal(data, &payload)
		blk := blocks[payload.Index]
		if blk != nil && blk.kind == "tool_use" {
			// Prefer the accumulated input_json_delta payload when present;
			// the content_block_start's initial "input" is an empty placeholder
			// on real Anthropic streams.
			if blk.args.Len() > 0 {
				s := strings.TrimSpace(blk.args.String())
				if json.Valid([]byte(s)) {
					blk.input = json.RawMessage(s)
				} else if s != "" {
					blk.input = json.RawMessage(fmt.Sprintf("%q", s))
				}
			} else if len(blk.input) == 0 {
				blk.input = json.RawMessage("{}")
			}
		}
		return nil

	case "message_delta":
		var payload struct {
			Delta struct {
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				OutputTokens             int `json:"output_tokens"`
				InputTokens              int `json:"input_tokens"`
				CacheReadInputTokens     int `json:"cache_read_input_tokens"`
				CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("anthropic: message_delta decode: %w", err)
		}
		*stopReason = mapStopReason(payload.Delta.StopReason)
		if payload.Usage.OutputTokens > 0 {
			usage.OutputTokens = payload.Usage.OutputTokens
		}
		if payload.Usage.InputTokens > 0 {
			usage.InputTokens = payload.Usage.InputTokens
		}
		if payload.Usage.CacheReadInputTokens > 0 {
			usage.CacheReadTokens = payload.Usage.CacheReadInputTokens
		}
		if payload.Usage.CacheCreationInputTokens > 0 {
			usage.CacheWriteTokens = payload.Usage.CacheCreationInputTokens
		}
		return nil

	case "message_stop":
		return nil

	case "ping":
		return nil

	case "error":
		var payload struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &payload)
		return fmt.Errorf("anthropic: stream error %s: %s", payload.Error.Type, payload.Error.Message)

	default:
		return nil
	}
}

func mapStopReason(s string) llm.StopReason {
	switch s {
	case "end_turn":
		return llm.StopReasonEndTurn
	case "tool_use":
		return llm.StopReasonToolUse
	case "max_tokens":
		return llm.StopReasonMaxTokens
	case "refusal":
		return llm.StopReasonRefusal
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
