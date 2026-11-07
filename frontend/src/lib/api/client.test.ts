import { afterEach, describe, expect, it, vi } from "vitest"

import { ApiError, apiFetch } from "./client"

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

describe("apiFetch", () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it("keeps existing_id from a duplicate-name 409", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse(409, {
          message_key: "error.food.duplicate_name",
          status: 409,
          existing_id: 42,
        })
      )
    )

    const err = await apiFetch("/foods", { method: "POST" }).catch((e) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({
      status: 409,
      messageKey: "error.food.duplicate_name",
      existingId: 42,
    })
  })

  it("turns a failed fetch into an error.network ApiError", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("fetch")))

    const err = await apiFetch<never>("/foods/1").catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 0, messageKey: "error.network" })
  })

  it("lets an abort through untouched", async () => {
    const abort = new DOMException("aborted", "AbortError")
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(abort))

    await expect(apiFetch<never>("/foods/1")).rejects.toBe(abort)
  })

  it("leaves existingId unset when the body has none", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          jsonResponse(404, { message_key: "error.not_found", status: 404 })
        )
    )

    const err = await apiFetch<never>("/foods/1").catch((e: ApiError) => e)
    expect(err).toMatchObject({ status: 404, messageKey: "error.not_found" })
    expect(err.existingId).toBeUndefined()
  })
})
