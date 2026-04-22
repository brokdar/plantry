// Package fake is a test/e2e-only LLM provider. It reads a scripted event
// sequence from a JSON file and streams those events back to the agent loop
// with optional realistic per-event delays. Activated via
// PLANTRY_AI_PROVIDER=fake + PLANTRY_AI_FAKE_SCRIPT=<path>.
package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// Script is the file-backed scenario the fake provider replays. Each call to
// Stream consumes one Turn; additional calls beyond len(Turns) return an
// error.
type Script struct {
	Turns []Turn `json:"turns"`
}

// Turn is a single assistant turn in the script.
type Turn struct {
	Events     []ScriptedEvent `json:"events"`
	StopReason string          `json:"stop_reason"` // "end_turn" | "tool_use" | ...
	// Message is the assembled final assistant message for this turn. Its
	// content blocks are what the loop will see; events are for UI only.
	Message ScriptedMessage `json:"message"`
	Usage   ScriptedUsage   `json:"usage"`
}

// ScriptedEvent is one canonical Event to emit, with optional pre-delay.
type ScriptedEvent struct {
	DelayMs int             `json:"delay_ms"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// ScriptedMessage is the assembled assistant message the loop will treat as
// the turn's result.
type ScriptedMessage struct {
	Content []ScriptedBlock `json:"content"`
}

// ScriptedBlock mirrors llm.ContentBlock with convenient field names.
type ScriptedBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	ToolName  string          `json:"tool_name,omitempty"`
	ToolInput json.RawMessage `json:"tool_input,omitempty"`
}

// ScriptedUsage matches llm.Usage.
type ScriptedUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Client implements llm.Client backed by a scripted file.
//
// Turn selection is per-invocation: a tool-use turn advances to the next turn
// on the next Stream() call, and the very last turn is reused indefinitely.
// This keeps chat sessions self-terminating (so a test doesn't hang) while
// allowing later chat requests in the same process to replay the script.
type Client struct {
	path   string
	script *Script
	call   int
	mu     sync.Mutex
}

// New loads the script and returns a Client. Returns an error if the file
// can't be read or parsed.
func New(path string) (*Client, error) {
	c := &Client{path: path}
	if err := c.reload(); err != nil {
		return nil, err
	}
	return c, nil
}

// NewFromScript creates a Client from an already-loaded Script. Handy for tests.
func NewFromScript(s *Script) *Client { return &Client{script: s} }

func (c *Client) reload() error {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return fmt.Errorf("fake: read script: %w", err)
	}
	var s Script
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("fake: parse script: %w", err)
	}
	c.script = &s
	return nil
}

// Stream replays the next scripted turn. The final turn is reused for any
// subsequent calls, so later chat sessions still see a valid terminal turn
// without needing to reload the script.
func (c *Client) Stream(ctx context.Context, _ llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	defer close(out)
	c.mu.Lock()
	if c.script == nil || len(c.script.Turns) == 0 {
		c.mu.Unlock()
		return nil, fmt.Errorf("fake: empty script")
	}
	idx := c.call
	if idx >= len(c.script.Turns) {
		idx = len(c.script.Turns) - 1
	}
	turn := c.script.Turns[idx]
	// Advance to the next turn for subsequent calls.
	if c.call < len(c.script.Turns) {
		c.call++
	}
	// If we just returned the last scripted turn and it is a terminal turn
	// (end_turn / empty stop_reason), reset so the next chat session starts
	// fresh. Non-terminal (tool_use) turns keep advancing inside one session.
	if turn.StopReason == "" || turn.StopReason == "end_turn" {
		c.call = 0
	}
	c.mu.Unlock()

	for _, e := range turn.Events {
		if e.DelayMs > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(e.DelayMs) * time.Millisecond):
			}
		}
		payload, err := decodePayload(e.Type, e.Payload)
		if err != nil {
			return nil, fmt.Errorf("fake: event %d %s: %w", c.call, e.Type, err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case out <- llm.Event{Type: llm.EventType(e.Type), Payload: payload}:
		}
	}
	return &llm.Response{
		Message:    assembleMessage(turn.Message),
		StopReason: mapStopReason(turn.StopReason),
		Usage:      llm.Usage{InputTokens: turn.Usage.InputTokens, OutputTokens: turn.Usage.OutputTokens},
	}, nil
}

func decodePayload(evtType string, raw json.RawMessage) (any, error) {
	// Decode based on event type so downstream consumers can type-assert cleanly.
	switch llm.EventType(evtType) {
	case llm.EventMessageStart:
		var p llm.MessageStartPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		return p, nil
	case llm.EventAssistantDelta:
		var p llm.AssistantDeltaPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		return p, nil
	case llm.EventToolCallStart:
		var p llm.ToolCallStartPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		return p, nil
	case llm.EventToolCallDelta:
		var p llm.ToolCallDeltaPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		return p, nil
	default:
		// Unknown types flow through with the raw payload for forward compat.
		return raw, nil
	}
}

func assembleMessage(m ScriptedMessage) llm.Message {
	content := make([]llm.ContentBlock, len(m.Content))
	for i, b := range m.Content {
		switch b.Type {
		case "tool_use":
			input := b.ToolInput
			if len(input) == 0 {
				input = json.RawMessage("{}")
			}
			content[i] = llm.ContentBlock{
				Type: llm.ContentTypeToolUse, ToolUseID: b.ToolUseID,
				ToolUseName: b.ToolName, ToolUseInput: input,
			}
		default:
			content[i] = llm.ContentBlock{Type: llm.ContentTypeText, Text: b.Text}
		}
	}
	return llm.Message{Role: llm.RoleAssistant, Content: content}
}

func mapStopReason(s string) llm.StopReason {
	switch s {
	case "tool_use":
		return llm.StopReasonToolUse
	case "max_tokens":
		return llm.StopReasonMaxTokens
	case "refusal":
		return llm.StopReasonRefusal
	default:
		return llm.StopReasonEndTurn
	}
}
