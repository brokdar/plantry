// External store for the assistant: the in-flight turn (text, status line),
// the changes the agent applied, a pending unlock request and the
// per-line undo state. Bypasses TanStack Query so per-token deltas don't cause
// a re-render storm; read via useSyncExternalStore. Server data (conversations)
// stays in TanStack Query. Open/closed lives in the URL (D15), not here.

import { useSyncExternalStore } from "react"

import type { ToolFailure } from "../assistant/transcript"
import type { Change, UnlockRequestPayload } from "../domain/assistantEvents"

/** State of one change line: `undoing` while the request runs, `undone` once
 *  reverted, `stale` when later edits make the revert unsafe. */
export type ChangeLineState = "undoing" | "undone" | "stale"

export interface AssistantStreamState {
  text: string
  /** Name of the tool currently executing, or null. */
  status: string | null
  changes: Change[]
  /** Tool calls of the turn in flight that failed. */
  failures: ToolFailure[]
  unlockRequest: UnlockRequestPayload | null
  lineStates: Record<number, ChangeLineState>
  conversationId: number | null
  draft: string
}

const initial: AssistantStreamState = {
  text: "",
  status: null,
  changes: [],
  failures: [],
  unlockRequest: null,
  lineStates: {},
  conversationId: null,
  draft: "",
}

let state: AssistantStreamState = initial
const listeners = new Set<() => void>()

function set(patch: Partial<AssistantStreamState>) {
  state = { ...state, ...patch }
  for (const l of listeners) l()
}

export const assistantStream = {
  get(): AssistantStreamState {
    return state
  },
  subscribe(l: () => void) {
    listeners.add(l)
    return () => listeners.delete(l)
  },
  /** Start of a new run: clears the turn, keeps conversation, draft and the
   *  change lines' states (a stale line must not offer Undo again). */
  reset() {
    set({
      text: "",
      status: null,
      changes: [],
      failures: [],
      unlockRequest: null,
    })
  },
  /** Clears the in-flight text only (each message_start) so earlier
   *  changes of the same run stay visible. */
  resetText() {
    set({ text: "" })
  },
  appendText(delta: string) {
    set({ text: state.text + delta })
  },
  setStatus(status: string | null) {
    set({ status })
  },
  addChange(change: Change) {
    if (state.changes.some((c) => c.id === change.id)) return
    set({ changes: [...state.changes, change] })
  },
  addFailure(failure: ToolFailure) {
    if (state.failures.some((f) => f.id === failure.id)) return
    set({ failures: [...state.failures, failure] })
  },
  setUnlockRequest(req: UnlockRequestPayload | null) {
    set({ unlockRequest: req })
  },
  setLineState(id: number, line: ChangeLineState | null) {
    const next = { ...state.lineStates }
    if (line === null) delete next[id]
    else next[id] = line
    set({ lineStates: next })
  },
  setConversation(id: number | null) {
    set({ conversationId: id })
  },
  setDraft(draft: string) {
    set({ draft })
  },
  /** Back to the empty assistant (tests, new conversation). */
  clear() {
    state = initial
    for (const l of listeners) l()
  },
}

export function useAssistantStreamState(): AssistantStreamState {
  return useSyncExternalStore(assistantStream.subscribe, assistantStream.get)
}

const noChange = null

/** The newest change of the current turn (the board pulses its cells, D17). */
export function useLastChange(): Change | null {
  return useSyncExternalStore(assistantStream.subscribe, () => {
    return state.changes.at(-1) ?? noChange
  })
}
