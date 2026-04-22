import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useCallback, useRef } from "react"

import {
  deleteConversation,
  getAISettings,
  getConversation,
  listConversations,
  postChatStream,
} from "../api/ai"
import type { ChatEvent, ChatRequest } from "../domain/chatEvents"
import { chatStreamStore } from "../stores/chat-stream"
import { useChatUI } from "../stores/chat-ui"

import { aiKeys, weekKeys } from "./keys"

export function useAISettings() {
  return useQuery({
    queryKey: aiKeys.settings(),
    queryFn: getAISettings,
    staleTime: 60_000,
  })
}

export function useConversations(weekId?: number) {
  return useQuery({
    queryKey: aiKeys.conversations(weekId),
    queryFn: () => listConversations(weekId),
    staleTime: 15_000,
  })
}

export function useConversation(id: number | null) {
  return useQuery({
    queryKey: id != null ? aiKeys.conversation(id) : ["ai", "conversation", -1],
    queryFn: () => getConversation(id!),
    enabled: id != null,
  })
}

export function useDeleteConversation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => deleteConversation(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: aiKeys.all })
    },
  })
}

/**
 * useChatStream owns the POST SSE reader lifetime. Calling .mutateAsync()
 * starts a fresh stream; the in-flight assistant turn is mirrored into the
 * chatStreamStore for low-overhead per-delta rendering. plate_changed
 * events invalidate week-scoped query caches immediately. Turn boundaries
 * and final completion flush to the conversation query cache.
 */
export interface ChatStreamParams {
  conversationId?: number
  weekId?: number
  mode?: "fill_empty" | "replace_all" | ""
  message: string
}

export function useChatStream() {
  const qc = useQueryClient()
  const controllerRef = useRef<AbortController | null>(null)

  const abort = useCallback(() => {
    controllerRef.current?.abort()
    controllerRef.current = null
  }, [])

  const mutation = useMutation({
    mutationFn: async (params: ChatStreamParams) => {
      controllerRef.current?.abort()
      const ac = new AbortController()
      controllerRef.current = ac

      chatStreamStore.reset()

      const req: ChatRequest = {
        conversation_id: params.conversationId,
        week_id: params.weekId,
        mode: params.mode,
        message: params.message,
      }

      try {
        for await (const evt of postChatStream(req, ac.signal)) {
          handleEvent(evt, qc, params.weekId)
        }
      } finally {
        if (controllerRef.current === ac) controllerRef.current = null
      }

      // Do NOT reset the stream buffer here — keep the last turn's tool
      // cards + text visible in the panel after done so the user (and tests)
      // can see what happened. The buffer is reset at the start of the next
      // mutation. Refetch conversations so the UI shows the fresh transcript.
      await qc.invalidateQueries({ queryKey: aiKeys.all })
    },
  })

  return {
    ...mutation,
    abort,
    isStreaming: mutation.isPending,
  }
}

function handleEvent(
  evt: ChatEvent,
  qc: ReturnType<typeof useQueryClient>,
  weekId?: number
) {
  switch (evt.type) {
    case "conversation_ready":
      useChatUI.getState().setActiveConversation(evt.data.conversation_id)
      return
    case "message_start":
      // Clear only the in-flight text so tool cards from earlier agent
      // iterations in this chat session remain visible.
      chatStreamStore.resetText()
      return
    case "assistant_delta":
      chatStreamStore.appendText(evt.data.text)
      return
    case "tool_call_start":
      chatStreamStore.startToolCall(evt.data.id, evt.data.name)
      return
    case "tool_call_delta":
      chatStreamStore.appendToolArgs(evt.data.id, evt.data.args_json)
      return
    case "tool_exec_start":
      chatStreamStore.setToolStatus(evt.data.id, "running")
      return
    case "tool_exec_end":
      chatStreamStore.setToolStatus(evt.data.id, evt.data.status, {
        durationMs: evt.data.duration_ms,
      })
      return
    case "tool_result":
      // Nothing to render beyond status; keep summary compact.
      return
    case "plate_changed": {
      const targetWeek = evt.data.week_id ?? weekId
      if (targetWeek) {
        qc.invalidateQueries({ queryKey: weekKeys.byId(targetWeek) })
        qc.invalidateQueries({ queryKey: weekKeys.nutrition(targetWeek) })
        qc.invalidateQueries({ queryKey: weekKeys.shoppingList(targetWeek) })
      } else {
        qc.invalidateQueries({ queryKey: weekKeys.all })
      }
      return
    }
    case "done":
      return
    case "error":
      return
  }
}
