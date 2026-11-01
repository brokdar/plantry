import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"
import { useCallback, useRef } from "react"
import { useTranslation } from "react-i18next"

import {
  deleteConversation,
  getConversation,
  listConversations,
  postChatStream,
  undoChange,
} from "../api/ai"
import { ApiError } from "../api/client"
import { firstUserText, toolFailed } from "../assistant/transcript"
import type { ChatEvent, ChatRequest } from "../domain/assistantEvents"
import { assistantStream } from "../stores/assistant-stream"

import { todayISO } from "@/lib/dates"
import { toastError } from "@/lib/toast"

import { aiKeys } from "./keys"
import { invalidatePlanViews } from "./plan"

export function useConversations() {
  return useQuery({
    queryKey: aiKeys.conversations(),
    queryFn: () => listConversations(),
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

/** A short title per conversation: its first user message, truncated. The
 *  list endpoint carries no title, so each detail is read (and cached). */
export function useConversationTitles(ids: number[]): Map<number, string> {
  const results = useQueries({
    queries: ids.map((id) => ({
      queryKey: aiKeys.conversation(id),
      queryFn: () => getConversation(id),
      staleTime: 60_000,
    })),
  })
  const titles = new Map<number, string>()
  results.forEach((r, i) => {
    const text = r.data ? firstUserText(r.data.messages) : ""
    if (text) titles.set(ids[i], text)
  })
  return titles
}

export function useDeleteConversation() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (id: number) => deleteConversation(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: aiKeys.all })
    },
    onError: (err) => toastError(err, t),
  })
}

/**
 * useAssistantStream owns the POST SSE reader lifetime. Calling .mutateAsync()
 * starts a fresh stream; the in-flight turn is mirrored into the
 * assistantStream store for low-overhead per-delta rendering. A `change`
 * event is appended to the store and invalidates the plan views (plan, Today,
 * shopping) once. The conversation query refetches when the stream ends.
 */
export interface AssistantStreamParams {
  conversationId?: number
  /** Start of the window the board shows. */
  start: string
  message: string
}

export function useAssistantStream() {
  const qc = useQueryClient()
  const controllerRef = useRef<AbortController | null>(null)

  const abort = useCallback(() => {
    controllerRef.current?.abort()
    controllerRef.current = null
  }, [])

  const mutation = useMutation({
    mutationFn: async (params: AssistantStreamParams) => {
      controllerRef.current?.abort()
      const ac = new AbortController()
      controllerRef.current = ac

      assistantStream.reset()

      const req: ChatRequest = {
        conversation_id: params.conversationId,
        message: params.message,
        today: todayISO(),
        start: params.start,
      }

      try {
        for await (const evt of postChatStream(req, ac.signal)) {
          handleEvent(evt, qc)
        }
      } finally {
        assistantStream.setStatus(null)
        if (controllerRef.current === ac) controllerRef.current = null
      }

      // The buffer is not reset here: the last turn's tool calls and change
      // lines stay visible until the next run starts.
      await qc.invalidateQueries({ queryKey: aiKeys.all })
    },
  })

  return {
    ...mutation,
    abort,
    isStreaming: mutation.isPending,
  }
}

/** Reverts one change. Maps the line state: 409 `change_stale` -> `stale`,
 *  409 `change_undone` -> `undone`; other errors toast. */
export function useUndoChange() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: async (id: number) => {
      assistantStream.setLineState(id, "undoing")
      try {
        return await undoChange(id)
      } catch (err) {
        if (err instanceof ApiError && err.status === 409) {
          if (err.messageKey === "error.ai.change_stale") {
            assistantStream.setLineState(id, "stale")
            return null
          }
          if (err.messageKey === "error.ai.change_undone") {
            assistantStream.setLineState(id, "undone")
            return null
          }
        }
        assistantStream.setLineState(id, null)
        throw err
      }
    },
    onSuccess: async (res, id) => {
      if (!res) return
      assistantStream.setLineState(id, "undone")
      await Promise.all([
        invalidatePlanViews(qc),
        qc.invalidateQueries({ queryKey: aiKeys.all }),
      ])
    },
    onError: (err) => toastError(err, t),
  })
}

function handleEvent(evt: ChatEvent, qc: ReturnType<typeof useQueryClient>) {
  switch (evt.type) {
    case "conversation_ready":
      assistantStream.setConversation(evt.data.conversation_id)
      return
    case "message_start":
      assistantStream.resetText()
      return
    case "assistant_delta":
      assistantStream.appendText(evt.data.text)
      return
    case "tool_call_start":
    case "tool_call_delta":
      return
    case "tool_exec_start":
      assistantStream.setStatus(evt.data.name)
      return
    case "tool_exec_end":
      assistantStream.setStatus(null)
      return
    case "tool_result":
      if (toolFailed(evt.data.is_error, evt.data.output)) {
        assistantStream.addFailure({ id: evt.data.id, tool: evt.data.name })
      }
      return
    case "change":
      assistantStream.addChange(evt.data)
      void invalidatePlanViews(qc)
      return
    case "unlock_request":
      assistantStream.setUnlockRequest(evt.data)
      return
    case "done":
    case "error":
      assistantStream.setStatus(null)
      return
  }
}
