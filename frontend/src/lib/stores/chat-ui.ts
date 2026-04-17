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

  setOpen: (open: boolean) => void
  setDraft: (draft: string) => void
  setActiveConversation: (id: number | null) => void
  setStreaming: (streaming: boolean) => void
  setMode: (mode: ChatMode) => void
  reset: () => void
}

export const useChatUI = create<ChatUIState>((set) => ({
  open: false,
  draftMessage: "",
  activeConversationId: null,
  streaming: false,
  mode: "",

  setOpen: (open) => set({ open }),
  setDraft: (draft) => set({ draftMessage: draft }),
  setActiveConversation: (id) => set({ activeConversationId: id }),
  setStreaming: (streaming) => set({ streaming }),
  setMode: (mode) => set({ mode }),
  reset: () =>
    set({
      draftMessage: "",
      streaming: false,
    }),
}))
