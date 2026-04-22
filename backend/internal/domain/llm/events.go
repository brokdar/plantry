package llm

import "encoding/json"

// EventType identifies a canonical streaming event produced by an LLM adapter
// or the agent loop. This vocabulary is vendor-neutral: adapters translate
// provider-specific wire events into these types.
type EventType string

const (
	// Emitted once at the start of a chat session after the conversation row
	// has been resolved or created, so the client can capture the id for
	// subsequent turns.
	EventConversationReady EventType = "conversation_ready"

	// Emitted once at the start of a new assistant turn.
	EventMessageStart EventType = "message_start"

	// Incremental text chunk from the assistant.
	EventAssistantDelta EventType = "assistant_delta"

	// First signal that the assistant is emitting a tool call (carries id + name).
	EventToolCallStart EventType = "tool_call_start"

	// Incremental JSON chunk for a tool call's input arguments.
	EventToolCallDelta EventType = "tool_call_delta"

	// Agent-loop-side: a tool is about to execute.
	EventToolExecStart EventType = "tool_exec_start"

	// Agent-loop-side: tool execution finished with ok|error status.
	EventToolExecEnd EventType = "tool_exec_end"

	// Agent-loop-side: the tool result payload, ready to feed back to the model.
	EventToolResult EventType = "tool_result"

	// Agent-loop-side: a tool just mutated the plan; the frontend should invalidate
	// week-scoped query caches (week, shopping-list, nutrition).
	EventPlateChanged EventType = "plate_changed"

	// Terminal success event.
	EventDone EventType = "done"

	// Terminal error event.
	EventError EventType = "error"
)

// Event is a single streaming event emitted to the agent loop or SSE client.
// Exactly one of the typed Payload fields on the receiver side must be read
// based on Type — see the payload struct types below.
type Event struct {
	Type    EventType
	Payload any
}

// ConversationReadyPayload accompanies EventConversationReady.
type ConversationReadyPayload struct {
	ConversationID int64 `json:"conversation_id"`
}

// MessageStartPayload accompanies EventMessageStart.
type MessageStartPayload struct {
	Model string `json:"model"`
}

// AssistantDeltaPayload accompanies EventAssistantDelta.
type AssistantDeltaPayload struct {
	Text string `json:"text"`
}

// ToolCallStartPayload accompanies EventToolCallStart.
type ToolCallStartPayload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ToolCallDeltaPayload accompanies EventToolCallDelta.
type ToolCallDeltaPayload struct {
	ID       string `json:"id"`
	ArgsJSON string `json:"args_json"`
}

// ToolExecStartPayload accompanies EventToolExecStart.
type ToolExecStartPayload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ToolExecStatus reports success/failure of a tool run.
type ToolExecStatus string

const (
	ToolExecStatusOK    ToolExecStatus = "ok"
	ToolExecStatusError ToolExecStatus = "error"
)

// ToolExecEndPayload accompanies EventToolExecEnd.
type ToolExecEndPayload struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Status     ToolExecStatus `json:"status"`
	DurationMs int64          `json:"duration_ms"`
}

// ToolResultPayload accompanies EventToolResult.
type ToolResultPayload struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Output  json.RawMessage `json:"output"`
	IsError bool            `json:"is_error"`
}

// PlateChangedPayload accompanies EventPlateChanged.
// WeekID, if nonzero, lets the frontend target invalidation precisely.
type PlateChangedPayload struct {
	WeekID int64 `json:"week_id,omitempty"`
}

// DonePayload accompanies EventDone.
type DonePayload struct {
	StopReason     StopReason `json:"stop_reason"`
	Usage          Usage      `json:"usage"`
	IterationCount int        `json:"iteration_count"`
}

// StreamErrorPayload accompanies EventError.
type StreamErrorPayload struct {
	MessageKey string `json:"message_key"`
	Message    string `json:"message"`
}
