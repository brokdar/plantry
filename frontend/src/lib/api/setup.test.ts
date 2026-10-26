import { afterEach, describe, expect, it, vi } from "vitest"

import { completeSetup, getSetupStatus } from "./setup"

const status = { completed: false, recipe_count: 3, recipe_target: 10 }

function mockFetch() {
  const fn = vi
    .fn()
    .mockResolvedValue(new Response(JSON.stringify(status), { status: 200 }))
  vi.stubGlobal("fetch", fn)
  return fn
}

describe("setup api", () => {
  afterEach(() => vi.unstubAllGlobals())

  it("GET /api/setup", async () => {
    const fetchFn = mockFetch()
    await expect(getSetupStatus()).resolves.toEqual(status)
    expect(fetchFn).toHaveBeenCalledWith("/api/setup", expect.anything())
  })

  it("POST /api/setup/complete", async () => {
    const fetchFn = mockFetch()
    await completeSetup()
    const [url, init] = fetchFn.mock.calls[0]
    expect(url).toBe("/api/setup/complete")
    expect(init.method).toBe("POST")
  })
})
