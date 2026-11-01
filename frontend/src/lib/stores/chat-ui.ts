// Ephemeral chat UI state. Persistent chat history lives in TanStack Query
// (qk.ai.conversation), not here.

import { create } from "zustand"

interface ChatUIState {
  open: boolean
  draftMessage: string
  activeConversationId: number | null
  streaming: boolean

  setOpen: (open: boolean) => void
  setDraft: (draft: string) => void
  setActiveConversation: (id: number | null) => void
  setStreaming: (streaming: boolean) => void
  reset: () => void
}

export const useChatUI = create<ChatUIState>((set) => ({
  open: false,
  draftMessage: "",
  activeConversationId: null,
  streaming: false,

  setOpen: (open) => set({ open }),
  setDraft: (draft) => set({ draftMessage: draft }),
  setActiveConversation: (id) => set({ activeConversationId: id }),
  setStreaming: (streaming) => set({ streaming }),
  reset: () =>
    set({
      draftMessage: "",
      streaming: false,
    }),
}))
