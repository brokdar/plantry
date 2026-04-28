// Canonical streaming event vocabulary — mirrors Go's internal/domain/llm/events.go.
// Every SSE frame from /api/ai/chat carries one of these event types and a
// matching payload shape.

export type ChatEventType =
  | "conversation_ready"
  | "message_start"
  | "assistant_delta"
  | "tool_call_start"
  | "tool_call_delta"
  | "tool_exec_start"
  | "tool_exec_end"
  | "tool_result"
  | "plate_changed"
  | "done"
  | "error"

export interface ConversationReadyPayload {
  conversation_id: number
}
export interface MessageStartPayload {
  model: string
}
export interface AssistantDeltaPayload {
  text: string
}
export interface ToolCallStartPayload {
  id: string
  name: string
}
export interface ToolCallDeltaPayload {
  id: string
  args_json: string
}
export interface ToolExecStartPayload {
  id: string
  name: string
}
export interface ToolExecEndPayload {
  id: string
  name: string
  status: "ok" | "error"
  duration_ms: number
}
export interface ToolResultPayload {
  id: string
  name: string
  output: unknown
  is_error: boolean
}
export interface PlateChangedPayload {
  date?: string
}
export interface DonePayload {
  stop_reason: string
  usage?: {
    input_tokens?: number
    output_tokens?: number
    cache_read_tokens?: number
  }
  iteration_count: number
}
export interface StreamErrorPayload {
  message_key: string
  message: string
}

export type ChatEvent =
  | { type: "conversation_ready"; data: ConversationReadyPayload }
  | { type: "message_start"; data: MessageStartPayload }
  | { type: "assistant_delta"; data: AssistantDeltaPayload }
  | { type: "tool_call_start"; data: ToolCallStartPayload }
  | { type: "tool_call_delta"; data: ToolCallDeltaPayload }
  | { type: "tool_exec_start"; data: ToolExecStartPayload }
  | { type: "tool_exec_end"; data: ToolExecEndPayload }
  | { type: "tool_result"; data: ToolResultPayload }
  | { type: "plate_changed"; data: PlateChangedPayload }
  | { type: "done"; data: DonePayload }
  | { type: "error"; data: StreamErrorPayload }

export interface ChatRequest {
  conversation_id?: number
  mode?: "fill_empty" | "replace_all" | ""
  message: string
}

export interface ConversationSummary {
  id: number
  title?: string
  created_at: string
  updated_at: string
}

export interface ConversationMessage {
  id: number
  role: "system" | "user" | "assistant" | "tool" | "error"
  content: unknown
  created_at: string
}

export interface ConversationDetail extends ConversationSummary {
  messages: ConversationMessage[]
}

export interface AISettings {
  enabled: boolean
  provider?: string
  model?: string
}
