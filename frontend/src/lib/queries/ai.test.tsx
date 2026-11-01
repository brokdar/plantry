import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import React from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ApiError } from "../api/client"
import { assistantStream } from "../stores/assistant-stream"

import { useAssistantStream, useUndoChange } from "./ai"
import { planKeys, todayKeys } from "./keys"

import { mockChange } from "@/test/fixtures"

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

const testStart = "2026-04-14"

function wrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

describe("useAssistantStream", () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    assistantStream.clear()
  })

  it("posts today and the window start with the message", async () => {
    vi.useFakeTimers({ toFake: ["Date"] })
    vi.setSystemTime(new Date(2026, 9, 26, 23, 30))
    try {
      const sse = `event: done\ndata: {"stop_reason":"end_turn","iteration_count":1}\n\n`
      const fetchMock = vi
        .fn()
        .mockResolvedValue(mockResponse(stringToReadable(sse)))
      vi.stubGlobal("fetch", fetchMock)

      const qc = new QueryClient({
        defaultOptions: { mutations: { retry: false } },
      })
      const { result } = renderHook(() => useAssistantStream(), {
        wrapper: wrapper(qc),
      })
      await act(async () => {
        await result.current.mutateAsync({
          message: "plan",
          start: "2026-10-31",
        })
      })

      expect(fetchMock).toHaveBeenCalledTimes(1)
      const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
      expect(url).toBe("/api/ai/chat")
      expect(JSON.parse(init.body as string)).toEqual({
        message: "plan",
        today: "2026-10-26",
        start: "2026-10-31",
      })
    } finally {
      vi.useRealTimers()
    }
  })

  it("routes deltas into assistantStream, not query cache", async () => {
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
    const { result } = renderHook(() => useAssistantStream(), {
      wrapper: wrapper(qc),
    })

    await act(async () => {
      await result.current.mutateAsync({ message: "hi", start: testStart })
    })

    // Buffer preserved after done so the UI can keep showing the last turn.
    expect(assistantStream.get().text).toBe("hi there")
    // No assistant_delta entries ever written to query cache.
    const all = qc.getQueryCache().findAll()
    expect(all.every((q) => !q.queryKey.includes("assistant_delta"))).toBe(true)
  })

  it("F1-AC2: a change event invalidates the plan views once and appends the change", async () => {
    const change = mockChange()
    const sse =
      `event: tool_exec_end\ndata: {"id":"tu_1","name":"set_session_portions","status":"ok","duration_ms":5}\n\n` +
      `event: change\ndata: ${JSON.stringify(change)}\n\n` +
      `event: unlock_request\ndata: {"rows":[{"date":"2026-04-15","slot_id":3,"names":["Chili"]}]}\n\n` +
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
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries")

    const { result } = renderHook(() => useAssistantStream(), {
      wrapper: wrapper(qc),
    })
    await act(async () => {
      await result.current.mutateAsync({ message: "plan", start: testStart })
    })

    const keys = invalidateSpy.mock.calls.map((c) => c[0]?.queryKey)
    expect(keys.filter((k) => k?.[0] === planKeys.all[0])).toEqual([
      planKeys.all,
    ])
    expect(keys).toContainEqual(todayKeys.all)
    expect(assistantStream.get().changes).toEqual([change])
    expect(assistantStream.get().unlockRequest?.rows[0].names).toEqual([
      "Chili",
    ])
  })

  it("tool_exec_start sets the status line, tool_exec_end clears it", async () => {
    const sse =
      `event: tool_call_start\ndata: {"id":"tu_1","name":"get_plan"}\n\n` +
      `event: tool_exec_start\ndata: {"id":"tu_1","name":"get_plan"}\n\n`
    const enc = new TextEncoder()
    const readable = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(enc.encode(sse))
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
    const statuses: Array<string | null> = []
    const unsub = assistantStream.subscribe(() =>
      statuses.push(assistantStream.get().status)
    )
    const { result } = renderHook(() => useAssistantStream(), {
      wrapper: wrapper(qc),
    })
    await act(async () => {
      await result.current.mutateAsync({ message: "x", start: testStart })
    })
    unsub()
    expect(statuses).toContain("get_plan")
    expect(assistantStream.get().status).toBeNull()
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
    const { result } = renderHook(() => useAssistantStream(), {
      wrapper: wrapper(qc),
    })

    let mutationPromise: Promise<unknown> | undefined
    act(() => {
      mutationPromise = result.current.mutateAsync({
        message: "x",
        start: testStart,
      })
    })
    await waitFor(() => expect(result.current.isStreaming).toBe(true))

    act(() => {
      result.current.abort()
    })

    await expect(mutationPromise).rejects.toBeDefined()
  })
})

describe("useUndoChange", () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    assistantStream.clear()
  })

  function run(res: Response) {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(res))
    const qc = new QueryClient({
      defaultOptions: { mutations: { retry: false } },
    })
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries")
    const hook = renderHook(() => useUndoChange(), { wrapper: wrapper(qc) })
    return { hook, invalidateSpy }
  }

  const json = (status: number, body: unknown) =>
    new Response(JSON.stringify(body), { status })

  it("undone: marks the line and invalidates the plan views", async () => {
    const { hook, invalidateSpy } = run(
      json(200, { plate_ids: [12], session_ids: [4] })
    )
    await act(async () => {
      await hook.result.current.mutateAsync(5)
    })
    expect(assistantStream.get().lineStates[5]).toBe("undone")
    const keys = invalidateSpy.mock.calls.map((c) => c[0]?.queryKey)
    expect(keys).toContainEqual(planKeys.all)
  })

  it("F1-AC3: 409 change_stale maps to the line state stale", async () => {
    const { hook, invalidateSpy } = run(
      json(409, { message_key: "error.ai.change_stale", status: 409 })
    )
    await act(async () => {
      await hook.result.current.mutateAsync(5)
    })
    expect(assistantStream.get().lineStates[5]).toBe("stale")
    expect(
      invalidateSpy.mock.calls.map((c) => c[0]?.queryKey)
    ).not.toContainEqual(planKeys.all)
  })

  it("409 change_undone maps to the line state undone", async () => {
    const { hook } = run(
      json(409, { message_key: "error.ai.change_undone", status: 409 })
    )
    await act(async () => {
      await hook.result.current.mutateAsync(5)
    })
    expect(assistantStream.get().lineStates[5]).toBe("undone")
  })

  it("other errors clear the line state and reject", async () => {
    const { hook } = run(
      json(404, { message_key: "error.not_found", status: 404 })
    )
    await act(async () => {
      await expect(hook.result.current.mutateAsync(5)).rejects.toBeInstanceOf(
        ApiError
      )
    })
    expect(assistantStream.get().lineStates[5]).toBeUndefined()
  })
})
