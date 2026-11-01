// Canonical assistant streaming event vocabulary — mirrors Go's internal/domain/llm/events.go.
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
  | "change"
  | "unlock_request"
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
/** One applied agent write: a display line plus what undo needs (Phase 4 §3.1).
 *  `params` carry display names and dates, never ids only. */
export interface Change {
  id: number
  run_id: number
  tool: string
  text_key: string
  params: Record<string, string | number | boolean | null>
  dates: string[]
  plate_ids: number[]
  session_ids: number[]
  undoable: boolean
}
export type ChangePayload = Change
export interface UnlockRequestRow {
  date: string
  slot_id: number
  names: string[]
}
export interface UnlockRequestPayload {
  rows: UnlockRequestRow[]
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
  | { type: "change"; data: ChangePayload }
  | { type: "unlock_request"; data: UnlockRequestPayload }
  | { type: "done"; data: DonePayload }
  | { type: "error"; data: StreamErrorPayload }

export interface ChatRequest {
  conversation_id?: number
  message: string
  /** The browser's date (YYYY-MM-DD). */
  today: string
  /** Start of the planning window the board shows (YYYY-MM-DD). */
  start: string
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

/** A change as stored on the conversation: `undone_at` is set once undone. */
export interface ConversationChange extends Change {
  undone_at: string | null
}

export interface ConversationDetail extends ConversationSummary {
  messages: ConversationMessage[]
  changes: ConversationChange[]
}

export interface UndoChangeResult {
  plate_ids: number[]
  session_ids: number[]
}
