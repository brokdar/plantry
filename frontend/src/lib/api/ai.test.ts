import { beforeEach, describe, expect, it, vi } from "vitest"

import { postChatStream } from "./ai"
import { ApiError } from "./client"

function stringToReadable(s: string): ReadableStream<Uint8Array> {
  const enc = new TextEncoder()
  return new ReadableStream({
    start(controller) {
      controller.enqueue(enc.encode(s))
      controller.close()
    },
  })
}

function chunksToReadable(chunks: string[]): ReadableStream<Uint8Array> {
  const enc = new TextEncoder()
  let i = 0
  return new ReadableStream({
    pull(controller) {
      if (i >= chunks.length) {
        controller.close()
        return
      }
      controller.enqueue(enc.encode(chunks[i++]))
    },
  })
}

function mockStreamResponse(body: ReadableStream<Uint8Array>): Response {
  return new Response(body, {
    status: 200,
    headers: { "Content-Type": "text/event-stream" },
  })
}

describe("postChatStream", () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it("yields canonical events in order", async () => {
    const sse =
      `event: message_start\ndata: {"model":"test"}\n\n` +
      `event: assistant_delta\ndata: {"text":"hi"}\n\n` +
      `event: done\ndata: {"stop_reason":"end_turn","iteration_count":1}\n\n`

    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(mockStreamResponse(stringToReadable(sse)))
    )

    const ac = new AbortController()
    const events: { type: string; data: unknown }[] = []
    for await (const e of postChatStream({ message: "hi" }, ac.signal)) {
      events.push({ type: e.type, data: e.data })
    }

    expect(events.map((e) => e.type)).toEqual([
      "message_start",
      "assistant_delta",
      "done",
    ])
    expect((events[1].data as { text: string }).text).toBe("hi")
  })

  it("reconstructs frames split across chunk boundaries", async () => {
    // Split an event in the middle of the payload.
    const chunks = [
      "event: assistant_delta\ndata: ",
      `{"text":"hel`,
      `lo"}\n\nevent: done\ndata: {"stop_reason":"end_turn","iteration_count":1}\n\n`,
    ]
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(mockStreamResponse(chunksToReadable(chunks)))
    )

    const events: { type: string; data: unknown }[] = []
    for await (const e of postChatStream(
      { message: "hi" },
      new AbortController().signal
    )) {
      events.push({ type: e.type, data: e.data })
    }
    expect(events[0].type).toBe("assistant_delta")
    expect((events[0].data as { text: string }).text).toBe("hello")
  })

  it("throws ApiError on non-OK response", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            message_key: "error.ai.provider_missing",
            status: 503,
          }),
          { status: 503, headers: { "Content-Type": "application/json" } }
        )
      )
    )

    await expect(async () => {
      for await (const evt of postChatStream(
        { message: "hi" },
        new AbortController().signal
      )) {
        void evt
      }
    }).rejects.toThrow(ApiError)
  })

  it("terminates after a done event without consuming trailing data", async () => {
    const sse =
      `event: done\ndata: {"stop_reason":"end_turn","iteration_count":1}\n\n` +
      `event: assistant_delta\ndata: {"text":"should-not-see"}\n\n`

    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(mockStreamResponse(stringToReadable(sse)))
    )

    const events: { type: string }[] = []
    for await (const e of postChatStream(
      { message: "x" },
      new AbortController().signal
    )) {
      events.push({ type: e.type })
    }
    expect(events.map((e) => e.type)).toEqual(["done"])
  })

  it("yields an error event and stops", async () => {
    const sse = `event: error\ndata: {"message_key":"error.ai.stream_interrupted","message":"boom"}\n\n`

    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(mockStreamResponse(stringToReadable(sse)))
    )

    const events: { type: string; data: unknown }[] = []
    for await (const e of postChatStream(
      { message: "x" },
      new AbortController().signal
    )) {
      events.push({ type: e.type, data: e.data })
    }
    expect(events).toHaveLength(1)
    expect(events[0].type).toBe("error")
  })
})
