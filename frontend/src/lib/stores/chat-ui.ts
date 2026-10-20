// Ephemeral chat UI state. Persistent chat history lives in TanStack Query
// (qk.ai.conversation), not here.

import { create } from "zustand"

export type ChatMode = "" | "fill_empty" | "replace_all"

interface ChatUIState {
  open: boolean
  draftMessage: string
  activeConversationId: number | null
  streaming: boolean
  mode: ChatMode
  // When set, the chat panel auto-submits this message once (used by the
  // "Fill empty slots" action so the assistant starts working immediately
  // instead of waiting for the user to press send). Cleared once consumed.
  autoSend: string | null

  setOpen: (open: boolean) => void
  setDraft: (draft: string) => void
  setActiveConversation: (id: number | null) => void
  setStreaming: (streaming: boolean) => void
  setMode: (mode: ChatMode) => void
  clearAutoSend: () => void
  reset: () => void
}

interface ChatUIStateExtended extends ChatUIState {
  openWith: (prefill: string) => void
  // Opens the panel in fill_empty mode and queues `message` for auto-submit.
  requestAutoFill: (message: string) => void
}

export const useChatUI = create<ChatUIStateExtended>((set) => ({
  open: false,
  draftMessage: "",
  activeConversationId: null,
  streaming: false,
  mode: "",
  autoSend: null,

  setOpen: (open) => set({ open }),
  setDraft: (draft) => set({ draftMessage: draft }),
  setActiveConversation: (id) => set({ activeConversationId: id }),
  setStreaming: (streaming) => set({ streaming }),
  setMode: (mode) => set({ mode }),
  clearAutoSend: () => set({ autoSend: null }),
  openWith: (prefill) => set({ open: true, draftMessage: prefill }),
  requestAutoFill: (message) =>
    set({
      open: true,
      mode: "fill_empty",
      draftMessage: "",
      autoSend: message,
    }),
  reset: () =>
    set({
      draftMessage: "",
      streaming: false,
      autoSend: null,
    }),
}))
