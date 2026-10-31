import { describe, expect, it } from "vitest"

import { ApiError } from "@/lib/api/client"

import { queryClient, retryTransient } from "./query-client"

describe("retryTransient", () => {
  it("never retries a 4xx ApiError", () => {
    for (const status of [400, 404, 409, 422]) {
      expect(retryTransient(0, new ApiError(status, "error.x"))).toBe(false)
    }
  })

  it("retries a 5xx ApiError and a network error up to 3 times", () => {
    for (const err of [new ApiError(500, "error.x"), new TypeError("fetch")]) {
      expect(retryTransient(0, err)).toBe(true)
      expect(retryTransient(2, err)).toBe(true)
      expect(retryTransient(3, err)).toBe(false)
    }
  })

  it("is the default for queries", () => {
    expect(queryClient.getDefaultOptions().queries?.retry).toBe(retryTransient)
  })
})
