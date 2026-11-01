// External store for the assistant: the in-flight turn (text, status line,
// tool calls), the changes the agent applied, a pending unlock request and the
// per-line undo state. Bypasses TanStack Query so per-token deltas don't cause
// a re-render storm; read via useSyncExternalStore. Server data (conversations)
// stays in TanStack Query. Open/closed lives in the URL (D15), not here.

import { useSyncExternalStore } from "react"

import type { Change, UnlockRequestPayload } from "../domain/assistantEvents"

export interface ToolCallState {
  id: string
  name: string
  argsJson: string
  status: "pending" | "running" | "ok" | "error"
  resultText?: string
  durationMs?: number
}

/** State of one change line: `undoing` while the request runs, `undone` once
 *  reverted, `stale` when later edits make the revert unsafe. */
export type ChangeLineState = "undoing" | "undone" | "stale"

export interface AssistantStreamState {
  text: string
  /** Name of the tool currently executing, or null. */
  status: string | null
  toolCalls: ToolCallState[]
  changes: Change[]
  unlockRequest: UnlockRequestPayload | null
  lineStates: Record<number, ChangeLineState>
  conversationId: number | null
  draft: string
}

const initial: AssistantStreamState = {
  text: "",
  status: null,
  toolCalls: [],
  changes: [],
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

function mapTool(id: string, fn: (tc: ToolCallState) => ToolCallState) {
  set({ toolCalls: state.toolCalls.map((tc) => (tc.id === id ? fn(tc) : tc)) })
}

export const assistantStream = {
  get(): AssistantStreamState {
    return state
  },
  subscribe(l: () => void) {
    listeners.add(l)
    return () => listeners.delete(l)
  },
  /** Start of a new run: clears the turn, keeps conversation and draft. */
  reset() {
    set({
      text: "",
      status: null,
      toolCalls: [],
      changes: [],
      unlockRequest: null,
      lineStates: {},
    })
  },
  /** Clears the in-flight text only (each message_start) so earlier tool calls
   *  and changes of the same run stay visible. */
  resetText() {
    set({ text: "" })
  },
  appendText(delta: string) {
    set({ text: state.text + delta })
  },
  setStatus(status: string | null) {
    set({ status })
  },
  startToolCall(id: string, name: string) {
    if (state.toolCalls.some((tc) => tc.id === id)) return
    set({
      toolCalls: [
        ...state.toolCalls,
        { id, name, argsJson: "", status: "pending" },
      ],
    })
  },
  appendToolArgs(id: string, chunk: string) {
    mapTool(id, (tc) => ({ ...tc, argsJson: tc.argsJson + chunk }))
  },
  setToolStatus(
    id: string,
    status: ToolCallState["status"],
    extras?: { resultText?: string; durationMs?: number }
  ) {
    mapTool(id, (tc) => ({ ...tc, status, ...extras }))
  },
  addChange(change: Change) {
    if (state.changes.some((c) => c.id === change.id)) return
    set({ changes: [...state.changes, change] })
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
