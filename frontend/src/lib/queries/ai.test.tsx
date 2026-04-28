import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import React from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { chatStreamStore } from "../stores/chat-stream"

import { useChatStream } from "./ai"
import { plateKeys } from "./keys"

function stringToReadable(s: string): ReadableStream<Uint8Array> {
  const enc = new TextEncoder()
  return new ReadableStream({
    start(controller) {
      controller.enqueue(enc.encode(s))
      controller.close()
    },
  })
}

function mockResponse(body: ReadableStream<Uint8Array>): Response {
  return new Response(body, {
    status: 200,
    headers: { "Content-Type": "text/event-stream" },
  })
}

function wrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

describe("useChatStream", () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    chatStreamStore.reset()
  })

  it("routes deltas into chatStreamStore, not query cache", async () => {
    const sse =
      `event: message_start\ndata: {"model":"test"}\n\n` +
      `event: assistant_delta\ndata: {"text":"hi "}\n\n` +
      `event: assistant_delta\ndata: {"text":"there"}\n\n` +
      `event: done\ndata: {"stop_reason":"end_turn","iteration_count":1}\n\n`

    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(mockResponse(stringToReadable(sse)))
    )

    const qc = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    const { result } = renderHook(() => useChatStream(), {
      wrapper: wrapper(qc),
    })

    await act(async () => {
      await result.current.mutateAsync({ message: "hi" })
    })

    // Buffer preserved after done so the UI can keep showing the last turn.
    expect(chatStreamStore.get().text).toBe("hi there")
    // No assistant_delta entries ever written to query cache.
    const all = qc.getQueryCache().findAll()
    expect(all.every((q) => !q.queryKey.includes("assistant_delta"))).toBe(true)
  })

  it("invalidates plate range cache on plate_changed", async () => {
    const from = "2026-04-14"
    const to = "2026-04-20"
    const sse =
      `event: tool_exec_end\ndata: {"id":"tu_1","name":"create_plate","status":"ok","duration_ms":5}\n\n` +
      `event: plate_changed\ndata: {}\n\n` +
      `event: done\ndata: {"stop_reason":"end_turn","iteration_count":1}\n\n`

    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(mockResponse(stringToReadable(sse)))
    )

    const qc = new QueryClient({
      defaultOptions: {
        queries: { retry: false, staleTime: Infinity },
        mutations: { retry: false },
      },
    })
    qc.setQueryData(plateKeys.range(from, to), { plates: [] })

    const invalidateSpy = vi.spyOn(qc, "invalidateQueries")

    const { result } = renderHook(() => useChatStream(), {
      wrapper: wrapper(qc),
    })
    await act(async () => {
      await result.current.mutateAsync({ message: "plan", range: { from, to } })
    })

    const plateInvalidations = invalidateSpy.mock.calls.filter(
      (c) =>
        Array.isArray(c[0]?.queryKey) &&
        (c[0]!.queryKey as readonly unknown[])[0] === "plates"
    )
    expect(plateInvalidations.length).toBeGreaterThanOrEqual(1)
  })

  it("tool_call_* events accumulate args in the stream buffer", async () => {
    const sse =
      `event: tool_call_start\ndata: {"id":"tu_1","name":"create_plate"}\n\n` +
      `event: tool_call_delta\ndata: {"id":"tu_1","args_json":"{\\"day\\":"}\n\n` +
      `event: tool_call_delta\ndata: {"id":"tu_1","args_json":"1}"}\n\n` +
      `event: tool_exec_start\ndata: {"id":"tu_1","name":"create_plate"}\n\n`

    // Custom stream that signals args accumulated before done — done never fires
    // so we check state during stream.
    const enc = new TextEncoder()
    const chunks = sse
      .split("\n\n")
      .filter((c) => c.length)
      .map((c) => c + "\n\n")

    const readable = new ReadableStream<Uint8Array>({
      start(controller) {
        chunks.forEach((c) => controller.enqueue(enc.encode(c)))
        // Don't emit done so the stream just ends (reader gets done=true).
        controller.close()
      },
    })
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(mockResponse(readable)))

    const qc = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })

    // Capture every snapshot so we can inspect the pre-final-reset state.
    const snapshots: Array<{
      toolCalls: Array<{ id: string; argsJson: string; status: string }>
    }> = []
    const unsub = chatStreamStore.subscribe(() => {
      const s = chatStreamStore.get()
      snapshots.push({
        toolCalls: s.toolCalls.map((tc) => ({
          id: tc.id,
          argsJson: tc.argsJson,
          status: tc.status,
        })),
      })
    })

    const { result } = renderHook(() => useChatStream(), {
      wrapper: wrapper(qc),
    })
    await act(async () => {
      await result.current.mutateAsync({ message: "x" })
    })
    unsub()

    // Find a snapshot where the tool call is fully accumulated + running.
    const match = snapshots.find(
      (s) =>
        s.toolCalls.length === 1 &&
        s.toolCalls[0].argsJson === '{"day":1}' &&
        s.toolCalls[0].status === "running"
    )
    expect(
      match,
      "expected a snapshot with full args + running status"
    ).toBeDefined()
    expect(match?.toolCalls[0].id).toBe("tu_1")
  })

  it("abort() cancels an in-flight stream", async () => {
    // Build a stream that closes (errors) when the fetch signal aborts, so
    // the reader's read() settles and the async generator terminates.
    const fetchSpy = vi.fn((_url: string, init?: RequestInit) => {
      const signal = init?.signal
      const readable = new ReadableStream<Uint8Array>({
        start(controller) {
          if (signal) {
            signal.addEventListener("abort", () => {
              controller.error(new DOMException("aborted", "AbortError"))
            })
          }
        },
      })
      return Promise.resolve(mockResponse(readable))
    })
    vi.stubGlobal("fetch", fetchSpy)

    const qc = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    const { result } = renderHook(() => useChatStream(), {
      wrapper: wrapper(qc),
    })

    let mutationPromise: Promise<unknown> | undefined
    act(() => {
      mutationPromise = result.current.mutateAsync({ message: "x" })
    })
    await waitFor(() => expect(result.current.isStreaming).toBe(true))

    act(() => {
      result.current.abort()
    })

    await expect(mutationPromise).rejects.toBeDefined()
  })
})
