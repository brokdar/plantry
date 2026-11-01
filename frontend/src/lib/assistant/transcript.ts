import type {
  ConversationChange,
  ConversationMessage,
  ToolResultChange,
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
const NOT_RECORDED = "change_not_recorded"

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
 *  acted on (`regenerate` makes `regenerate_side`). Only the fallback for
 *  results that carry no `change_id`. */
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
 * followed by the change lines its tool calls made. A tool result names the
 * change its call made (`change_id`), success or error alike; that is how
 * a change finds its call. Results without a `change_id` (stored before
 * the backend wrote it) fall back to matching the remaining changes
 * (ascending id) to the calls in order by tool name.
 */
export function buildTranscript(
  messages: ConversationMessage[],
  changes: ConversationChange[]
): Transcript {
  // Per call: the change its result names, whether it failed, and whether
  // it can have made a change the result does not name.
  const changeOf = new Map<string, number>()
  const noChange = new Set<string>()
  const failed = new Set<string>()
  let pendingUnlock: UnlockRequestPayload | null = null
  for (const m of messages) {
    if (m.role === "user") pendingUnlock = null
    for (const b of blocksOf(m)) {
      if (b.type !== "tool_result" || !b.tool_result_id) continue
      const result = parsedResult(b.tool_result_content) as
        | (Record<string, unknown> & ToolResultChange)
        | null
      const changeId = result?.change_id
      if (typeof changeId === "number") changeOf.set(b.tool_result_id, changeId)
      const preview = previewRows(b)
      if (preview) {
        noChange.add(b.tool_result_id)
        pendingUnlock = preview
        continue
      }
      if (toolFailed(!!b.tool_result_is_error, b.tool_result_content)) {
        failed.add(b.tool_result_id)
        noChange.add(b.tool_result_id)
      }
      // A change that was made but not saved has no change to show.
      if (result?.warning === NOT_RECORDED) noChange.add(b.tool_result_id)
    }
  }
  const byId = new Map(changes.map((c) => [c.id, c]))
  const claimed = new Set(changeOf.values())
  const queue = [...changes]
    .filter((c) => !claimed.has(c.id))
    .sort((a, b) => a.id - b.id)
  const placed = new Set<number>()
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
        }
        const named = byId.get(changeOf.get(id) ?? -1)
        if (named) {
          own.push(named)
          placed.add(named.id)
          continue
        }
        if (noChange.has(id) || changeOf.has(id)) continue
        const at = queue.findIndex((c) => sameTool(c.tool, b.tool_use_name))
        if (at >= 0) own.push(...queue.splice(at, 1))
      }
    }
    if (text === "" && own.length === 0 && failures.length === 0) continue
    turns.push({ message: m, text, changes: own, failures })
  }
  // Named by a result whose call is not in the conversation: shown last.
  for (const c of changes) {
    if (claimed.has(c.id) && !placed.has(c.id)) queue.push(c)
  }
  queue.sort((a, b) => a.id - b.id)
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
