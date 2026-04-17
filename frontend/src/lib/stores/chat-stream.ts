// External store for the currently-streaming assistant turn. Bypasses
// TanStack Query so that per-token delta events don't trigger a React
// re-render storm — the buffer is a plain mutable object + subscription
// set, read via useSyncExternalStore. At turn boundaries the completed
// turn is flushed into the TanStack Query cache by useChatStream.

import { useSyncExternalStore } from "react"

export interface ToolCallState {
  id: string
  name: string
  argsJson: string
  status: "pending" | "running" | "ok" | "error"
  resultText?: string
  durationMs?: number
}

export interface ChatStreamState {
  text: string
  toolCalls: ToolCallState[]
}

const emptyState: ChatStreamState = { text: "", toolCalls: [] }

let state: ChatStreamState = emptyState
const listeners = new Set<() => void>()

function emit() {
  for (const l of listeners) l()
}

export const chatStreamStore = {
  get(): ChatStreamState {
    return state
  },
  subscribe(l: () => void) {
    listeners.add(l)
    return () => listeners.delete(l)
  },
  reset() {
    state = { text: "", toolCalls: [] }
    emit()
  },
  // resetText clears the in-flight assistant text but keeps completed tool
  // calls. Used at each new agent turn (message_start) so text from the
  // previous turn doesn't concatenate into the next, while the tool-call
  // trail the user saw stays visible.
  resetText() {
    state = { ...state, text: "" }
    emit()
  },
  appendText(delta: string) {
    state = { ...state, text: state.text + delta }
    emit()
  },
  startToolCall(id: string, name: string) {
    if (state.toolCalls.some((tc) => tc.id === id)) return
    state = {
      ...state,
      toolCalls: [
        ...state.toolCalls,
        { id, name, argsJson: "", status: "pending" },
      ],
    }
    emit()
  },
  appendToolArgs(id: string, chunk: string) {
    state = {
      ...state,
      toolCalls: state.toolCalls.map((tc) =>
        tc.id === id ? { ...tc, argsJson: tc.argsJson + chunk } : tc
      ),
    }
    emit()
  },
  setToolStatus(
    id: string,
    status: ToolCallState["status"],
    extras?: { resultText?: string; durationMs?: number }
  ) {
    state = {
      ...state,
      toolCalls: state.toolCalls.map((tc) =>
        tc.id === id ? { ...tc, status, ...extras } : tc
      ),
    }
    emit()
  },
}

export function useChatStream(): ChatStreamState {
  return useSyncExternalStore(chatStreamStore.subscribe, chatStreamStore.get)
}
