import type {
  ConversationChange,
  ConversationMessage,
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
}

export interface Transcript {
  turns: TranscriptTurn[]
  /** Changes no turn's tool call could be matched to; shown last. */
  rest: ConversationChange[]
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

/** Whether a tool result recorded no change: an error, an unlock preview, or
 *  a run (regenerate) that ended in neither ok nor partial. */
function madeNoChange(b: Block): boolean {
  if (b.tool_result_is_error) return true
  const status = parsedResult(b.tool_result_content)?.status
  return (
    status === "needs_user_confirmation" ||
    (typeof status === "string" && status !== "ok" && status !== "partial")
  )
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
  for (const m of messages) {
    for (const b of blocksOf(m)) {
      if (b.type !== "tool_result" || !b.tool_result_id) continue
      if (madeNoChange(b)) noChange.add(b.tool_result_id)
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
    if (m.role === "assistant") {
      for (const b of blocks) {
        if (
          b.type === "tool_use" &&
          !noChange.has(b.tool_use_id ?? "") &&
          queue[0]?.tool === b.tool_use_name
        ) {
          own.push(queue.shift()!)
        }
      }
    }
    if (text === "" && own.length === 0) continue
    turns.push({ message: m, text, changes: own })
  }
  return { turns, rest: queue }
}
