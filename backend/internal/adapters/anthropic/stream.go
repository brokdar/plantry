package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
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

	st := &streamState{blocks: map[int]*blockAccum{}}

	flushEvent := func() error {
		defer func() {
			eventName = ""
			dataBuf.Reset()
		}()
		if dataBuf.Len() == 0 {
			return nil
		}
		data := bytes.TrimRight(dataBuf.Bytes(), "\n")
		return handleSSEEvent(ctx, eventName, data, out, st)
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
	content := make([]llm.ContentBlock, 0, len(st.order))
	for _, idx := range st.order {
		b, ok := st.blocks[idx]
		if !ok {
			continue
		}
		content = append(content, b.finalize())
	}
	return &llm.Response{
		Message:       llm.Message{Role: llm.RoleAssistant, Content: content},
		StopReason:    st.stopReason,
		Usage:         st.usage,
		SearchResults: st.results,
	}, nil
}

// streamState accumulates one streamed assistant turn.
type streamState struct {
	blocks     map[int]*blockAccum
	order      []int
	stopReason llm.StopReason
	usage      llm.Usage
	model      string
	results    []llm.SearchResult
}

// addResults appends the pages of a web_search_tool_result block, skipping
// URLs already seen. An error result (content is an object) adds nothing.
func (st *streamState) addResults(content json.RawMessage) {
	var pages []struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	}
	if json.Unmarshal(content, &pages) != nil {
		return
	}
	for _, p := range pages {
		if p.URL == "" || slices.ContainsFunc(st.results, func(r llm.SearchResult) bool { return r.URL == p.URL }) {
			continue
		}
		st.results = append(st.results, llm.SearchResult{URL: p.URL, Title: p.Title})
	}
}

// isServerBlock reports whether a block type is a server tool block
// (server_tool_use, web_search_tool_result, ...) that must be replayed
// verbatim.
func isServerBlock(kind string) bool {
	return kind == "server_tool_use" || (kind != "tool_result" && strings.HasSuffix(kind, "_tool_result"))
}

type blockAccum struct {
	kind      string // "text" | "tool_use" | server block type | other
	text      strings.Builder
	citations []string
	id        string
	name      string
	args      strings.Builder
	input     json.RawMessage
	raw       json.RawMessage // server blocks: the block JSON, compacted
}

func (b *blockAccum) finalize() llm.ContentBlock {
	if isServerBlock(b.kind) {
		return llm.ContentBlock{Type: llm.ContentTypeServer, Raw: b.raw}
	}
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
		return llm.ContentBlock{Type: llm.ContentTypeText, Text: b.text.String(), Citations: b.citations}
	}
}

func handleSSEEvent(
	ctx context.Context,
	name string,
	data []byte,
	out chan<- llm.Event,
	st *streamState,
) error {
	switch name {
	case "message_start":
		var payload struct {
			Message struct {
				Model string `json:"model"`
				Usage struct {
					InputTokens              int           `json:"input_tokens"`
					OutputTokens             int           `json:"output_tokens"`
					CacheCreationInputTokens int           `json:"cache_creation_input_tokens"`
					CacheReadInputTokens     int           `json:"cache_read_input_tokens"`
					ServerToolUse            serverToolUse `json:"server_tool_use"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("anthropic: message_start decode: %w", err)
		}
		st.model = payload.Message.Model
		st.usage.InputTokens = payload.Message.Usage.InputTokens
		st.usage.OutputTokens = payload.Message.Usage.OutputTokens
		st.usage.CacheReadTokens = payload.Message.Usage.CacheReadInputTokens
		st.usage.CacheWriteTokens = payload.Message.Usage.CacheCreationInputTokens
		st.usage.WebSearchRequests = payload.Message.Usage.ServerToolUse.WebSearchRequests
		return send(ctx, out, llm.Event{Type: llm.EventMessageStart, Payload: llm.MessageStartPayload{Model: st.model}})

	case "content_block_start":
		var payload struct {
			Index        int `json:"index"`
			ContentBlock struct {
				Type    string          `json:"type"`
				Text    string          `json:"text"`
				ID      string          `json:"id"`
				Name    string          `json:"name"`
				Input   json.RawMessage `json:"input"`
				Content json.RawMessage `json:"content"`
			} `json:"content_block"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("anthropic: content_block_start decode: %w", err)
		}
		blk := &blockAccum{kind: payload.ContentBlock.Type}
		if isServerBlock(blk.kind) {
			var raw struct {
				ContentBlock json.RawMessage `json:"content_block"`
			}
			var buf bytes.Buffer
			if err := json.Unmarshal(data, &raw); err != nil || json.Compact(&buf, raw.ContentBlock) != nil {
				return fmt.Errorf("anthropic: %s block decode", blk.kind)
			}
			blk.raw = buf.Bytes()
			if blk.kind == "web_search_tool_result" {
				st.addResults(payload.ContentBlock.Content)
			}
		}
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
		st.blocks[payload.Index] = blk
		st.order = append(st.order, payload.Index)
		return nil

	case "content_block_delta":
		var payload struct {
			Index int `json:"index"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
				Citation    struct {
					URL string `json:"url"`
				} `json:"citation"`
			} `json:"delta"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("anthropic: content_block_delta decode: %w", err)
		}
		blk := st.blocks[payload.Index]
		if blk == nil {
			return nil
		}
		switch payload.Delta.Type {
		case "text_delta":
			blk.text.WriteString(payload.Delta.Text)
			return send(ctx, out, llm.Event{Type: llm.EventAssistantDelta, Payload: llm.AssistantDeltaPayload{
				Text: payload.Delta.Text,
			}})
		case "citations_delta":
			if u := payload.Delta.Citation.URL; u != "" && !slices.Contains(blk.citations, u) {
				blk.citations = append(blk.citations, u)
			}
		case "input_json_delta":
			blk.args.WriteString(payload.Delta.PartialJSON)
			// Only client tool calls are surfaced; server tool input stays in
			// the block and other block types have no tool ID.
			if blk.kind != "tool_use" {
				return nil
			}
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
		blk := st.blocks[payload.Index]
		if blk != nil && blk.kind == "server_tool_use" && blk.args.Len() > 0 {
			return blk.setServerInput()
		}
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
				OutputTokens             int           `json:"output_tokens"`
				InputTokens              int           `json:"input_tokens"`
				CacheReadInputTokens     int           `json:"cache_read_input_tokens"`
				CacheCreationInputTokens int           `json:"cache_creation_input_tokens"`
				ServerToolUse            serverToolUse `json:"server_tool_use"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("anthropic: message_delta decode: %w", err)
		}
		st.stopReason = mapStopReason(payload.Delta.StopReason)
		if payload.Usage.OutputTokens > 0 {
			st.usage.OutputTokens = payload.Usage.OutputTokens
		}
		if payload.Usage.InputTokens > 0 {
			st.usage.InputTokens = payload.Usage.InputTokens
		}
		if payload.Usage.CacheReadInputTokens > 0 {
			st.usage.CacheReadTokens = payload.Usage.CacheReadInputTokens
		}
		if payload.Usage.CacheCreationInputTokens > 0 {
			st.usage.CacheWriteTokens = payload.Usage.CacheCreationInputTokens
		}
		if n := payload.Usage.ServerToolUse.WebSearchRequests; n > 0 {
			st.usage.WebSearchRequests = n
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
	case "pause_turn":
		return llm.StopReasonPauseTurn
	default:
		return llm.StopReason(s)
	}
}

// serverToolUse is the usage.server_tool_use object (cumulative counts).
type serverToolUse struct {
	WebSearchRequests int `json:"web_search_requests"`
}

// setServerInput writes the streamed input of a server_tool_use block into
// its raw JSON (the start event carries an empty placeholder).
func (b *blockAccum) setServerInput() error {
	args := strings.TrimSpace(b.args.String())
	if !json.Valid([]byte(args)) {
		return fmt.Errorf("anthropic: server_tool_use %s: invalid input", b.id)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b.raw, &fields); err != nil {
		return fmt.Errorf("anthropic: server_tool_use decode: %w", err)
	}
	fields["input"] = json.RawMessage(args)
	raw, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("anthropic: server_tool_use encode: %w", err)
	}
	b.raw = raw
	return nil
}

func send(ctx context.Context, out chan<- llm.Event, evt llm.Event) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case out <- evt:
		return nil
	}
}
