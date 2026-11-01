import type {
  ConversationChange,
  ConversationMessage,
  UnlockRequestPayload,
} from "@/lib/domain/assistantEvents"

interface Block {
  type?: string
  text?: string
  tool_use_id?: string
  tool_use_name?: string
  tool_result_id?: string
  tool_result_is_error?: boolean
  tool_result_content?: unknown
}

function blocksOf(m: ConversationMessage): Block[] {
  return Array.isArray(m.content) ? (m.content as Block[]) : []
}

export interface TranscriptTurn {
  message: ConversationMessage
  /** The turn's prose; tool calls and results never appear (D16). */
  text: string
  /** Changes made by the tool calls of this turn. */
  changes: ConversationChange[]
  /** Tool calls of this turn that failed (one failure line each). */
  failures: ToolFailure[]
}

/** A tool call that did not do what was asked: an error result, or a run
 *  that ended neither ok nor partial. */
export interface ToolFailure {
  /** The tool call's id. */
  id: string
  tool: string
}

export interface Transcript {
  turns: TranscriptTurn[]
  /** Changes no turn's tool call could be matched to; shown last. */
  rest: ConversationChange[]
  /** The unlock the assistant asked about and the user has not answered. */
  pendingUnlock: UnlockRequestPayload | null
}

/** A tool result's content as an object: the backend stores it as JSON,
 *  possibly still as text. */
function parsedResult(content: unknown): Record<string, unknown> | null {
  let v = content
  if (typeof v === "string") {
    try {
      v = JSON.parse(v)
    } catch {
      return null
    }
  }
  return v && typeof v === "object" ? (v as Record<string, unknown>) : null
}

const PREVIEW = "needs_user_confirmation"

/** Whether a tool result is a failure: an error, or a run (regenerate) that
 *  ended in neither ok nor partial. An unlock preview is neither. */
export function toolFailed(isError: boolean, content: unknown): boolean {
  if (isError) return true
  const status = parsedResult(content)?.status
  return (
    typeof status === "string" &&
    status !== PREVIEW &&
    status !== "ok" &&
    status !== "partial"
  )
}

/** A change's tool is its call's name, or the call's name plus the kind it
 *  acted on (`regenerate` makes `regenerate_side`). */
function sameTool(change: string, call: string | undefined): boolean {
  return (
    call !== undefined && (change === call || change.startsWith(call + "_"))
  )
}

/** The rows of an unlock preview result, or null for any other result. */
function previewRows(b: Block): UnlockRequestPayload | null {
  const r = parsedResult(b.tool_result_content)
  if (r?.status !== PREVIEW) return null
  return {
    rows: (Array.isArray(r.rows) ? r.rows : []) as UnlockRequestPayload["rows"],
  }
}

/**
 * The persisted conversation as the panel shows it: text turns, each
 * followed by the change lines its tool calls made. A change carries its
 * tool name but not its position, so changes (ascending id) are matched to
 * the successful tool calls in order by tool name.
 */
export function buildTranscript(
  messages: ConversationMessage[],
  changes: ConversationChange[]
): Transcript {
  // Calls that made no change: failed ones, an unlock's preview and a
  // regenerate that did not finish ok or partial.
  const noChange = new Set<string>()
  const failed = new Set<string>()
  let pendingUnlock: UnlockRequestPayload | null = null
  for (const m of messages) {
    if (m.role === "user") pendingUnlock = null
    for (const b of blocksOf(m)) {
      if (b.type !== "tool_result" || !b.tool_result_id) continue
      const preview = previewRows(b)
      if (preview) {
        noChange.add(b.tool_result_id)
        pendingUnlock = preview
      } else if (toolFailed(!!b.tool_result_is_error, b.tool_result_content)) {
        noChange.add(b.tool_result_id)
        failed.add(b.tool_result_id)
      }
    }
  }
  const queue = [...changes].sort((a, b) => a.id - b.id)
  const turns: TranscriptTurn[] = []
  for (const m of messages) {
    if (m.role === "tool") continue
    const blocks = blocksOf(m)
    const text = blocks
      .filter((b) => b.type === "text")
      .map((b) => b.text ?? "")
      .filter(Boolean)
      .join("\n\n")
    const own: ConversationChange[] = []
    const failures: ToolFailure[] = []
    if (m.role === "assistant") {
      for (const b of blocks) {
        if (b.type !== "tool_use") continue
        const id = b.tool_use_id ?? ""
        if (failed.has(id)) {
          failures.push({ id, tool: b.tool_use_name ?? "" })
          continue
        }
        if (noChange.has(id)) continue
        const at = queue.findIndex((c) => sameTool(c.tool, b.tool_use_name))
        if (at >= 0) own.push(...queue.splice(at, 1))
      }
    }
    if (text === "" && own.length === 0 && failures.length === 0) continue
    turns.push({ message: m, text, changes: own, failures })
  }
  return { turns, rest: queue, pendingUnlock }
}

const TITLE_MAX = 48

/** A conversation's first user message, collapsed to one line and cut to
 *  a short title; "" while it has none. */
export function firstUserText(messages: ConversationMessage[]): string {
  const first = messages.find((m) => m.role === "user")
  if (!first) return ""
  const text = blocksOf(first)
    .filter((b) => b.type === "text")
    .map((b) => b.text ?? "")
    .join(" ")
    .replace(/\s+/g, " ")
    .trim()
  return text.length > TITLE_MAX
    ? `${text.slice(0, TITLE_MAX - 1).trimEnd()}…`
    : text
}
