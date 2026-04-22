// Package llm defines the provider-agnostic LLM port and the canonical event
// vocabulary the agent loop speaks. Adapters (anthropic, openai, fake) translate
// their wire format into these types.
package llm

import (
	"context"
	"encoding/json"
)

// Role identifies the author of a message in a conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentType discriminates the body shape carried by a ContentBlock.
type ContentType string

const (
	ContentTypeText       ContentType = "text"
	ContentTypeToolUse    ContentType = "tool_use"
	ContentTypeToolResult ContentType = "tool_result"
)

// ContentBlock is a provider-neutral content item inside a Message.
// Exactly one of the shape fields is populated based on Type.
type ContentBlock struct {
	Type ContentType `json:"type"`

	// Text block payload.
	Text string `json:"text,omitempty"`

	// ToolUse block payload (assistant emitting a tool call).
	ToolUseID    string          `json:"tool_use_id,omitempty"`
	ToolUseName  string          `json:"tool_use_name,omitempty"`
	ToolUseInput json.RawMessage `json:"tool_use_input,omitempty"`

	// ToolResult block payload (user reporting back the tool output).
	ToolResultID      string          `json:"tool_result_id,omitempty"`
	ToolResultContent json.RawMessage `json:"tool_result_content,omitempty"`
	ToolResultIsError bool            `json:"tool_result_is_error,omitempty"`
}

// Message is a single turn in a conversation.
type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`
}

// Tool describes a callable tool the model may invoke.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
}

// Request is the provider-neutral prompt sent to a Client.
type Request struct {
	Model       string
	System      string
	Messages    []Message
	Tools       []Tool
	Temperature float64
	MaxTokens   int
}

// Usage reports token accounting for a single provider call.
type Usage struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
}

// StopReason identifies why the assistant turn ended.
type StopReason string

const (
	StopReasonEndTurn   StopReason = "end_turn"
	StopReasonToolUse   StopReason = "tool_use"
	StopReasonMaxTokens StopReason = "max_tokens"
	StopReasonRefusal   StopReason = "refusal"
	StopReasonError     StopReason = "error"
)

// Response is the final (non-streamed) shape of an assistant turn.
type Response struct {
	Message    Message
	StopReason StopReason
	Usage      Usage
}

// Client is the port an LLM provider adapter must implement.
//
// Stream streams the assistant turn as a sequence of canonical Events on out
// and returns the final (assembled) Response when the turn ends. The adapter
// closes out before returning. Stream must respect ctx cancellation and
// return promptly. The returned error is non-nil only for network/protocol
// failures; a successful turn that ended in tool_use still returns a non-nil
// Response whose StopReason == StopReasonToolUse.
type Client interface {
	Stream(ctx context.Context, req Request, out chan<- Event) (*Response, error)
}
