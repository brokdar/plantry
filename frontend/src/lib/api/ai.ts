import { EventSourceParserStream } from "eventsource-parser/stream"

import { apiFetch, ApiError } from "./client"
import type {
  AISettings,
  ChatEvent,
  ChatEventType,
  ChatRequest,
  ConversationDetail,
  ConversationSummary,
} from "../domain/chatEvents"

const BASE = "/api"

/**
 * postChatStream opens a POST SSE stream and yields canonical ChatEvents as
 * they arrive. The generator terminates when the server closes the stream
 * (after a `done` or `error` event) or when the AbortSignal fires.
 */
export async function* postChatStream(
  req: ChatRequest,
  signal: AbortSignal
): AsyncGenerator<ChatEvent, void, unknown> {
  const res = await fetch(`${BASE}/ai/chat`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
    },
    body: JSON.stringify(req),
    signal,
  })

  if (!res.ok) {
    let body: { message_key?: string; status?: number } = {}
    try {
      body = await res.json()
    } catch {
      // ignore
    }
    throw new ApiError(
      body.status ?? res.status,
      body.message_key ?? "error.server"
    )
  }
  if (!res.body) {
    throw new ApiError(res.status, "error.server")
  }

  const stream = res.body
    .pipeThrough(new TextDecoderStream())
    .pipeThrough(new EventSourceParserStream())

  const reader = stream.getReader()
  try {
    while (true) {
      const { value, done } = await reader.read()
      if (done) return
      if (!value || !value.event) continue
      let data: unknown
      try {
        data = JSON.parse(value.data)
      } catch {
        continue
      }
      yield { type: value.event as ChatEventType, data } as ChatEvent
      if (value.event === "done" || value.event === "error") return
    }
  } finally {
    reader.releaseLock()
  }
}

export function listConversations() {
  return apiFetch<{ items: ConversationSummary[]; total: number }>(
    "/ai/conversations"
  )
}

export function getConversation(id: number) {
  return apiFetch<ConversationDetail>(`/ai/conversations/${id}`)
}

export function deleteConversation(id: number) {
  return apiFetch<void>(`/ai/conversations/${id}`, { method: "DELETE" })
}

export function getAISettings() {
  return apiFetch<AISettings>("/settings/ai")
}
