import { afterEach, describe, expect, it, vi } from "vitest"

import { mockAvailabilityDefaults } from "@/test/fixtures"

import {
  clearAvailabilityOverride,
  getAvailabilityDefaults,
  getAvailabilityRange,
  setAvailabilityDefaults,
  setAvailabilityOverride,
} from "./availability"

function mockFetch(status = 200, body: unknown = { items: [] }) {
  const fn = vi
    .fn()
    .mockResolvedValue(
      status === 204
        ? new Response(null, { status })
        : new Response(JSON.stringify(body), { status })
    )
  vi.stubGlobal("fetch", fn)
  return fn
}

describe("availability api", () => {
  afterEach(() => vi.unstubAllGlobals())

  it("GET /api/availability/defaults", async () => {
    const fetchFn = mockFetch()
    await getAvailabilityDefaults()
    expect(fetchFn).toHaveBeenCalledWith(
      "/api/availability/defaults",
      expect.anything()
    )
  })

  it("PUT /api/availability/defaults with the full grid", async () => {
    const fetchFn = mockFetch()
    await setAvailabilityDefaults(mockAvailabilityDefaults)
    const [url, init] = fetchFn.mock.calls[0]
    expect(url).toBe("/api/availability/defaults")
    expect(init.method).toBe("PUT")
    expect(JSON.parse(init.body)).toEqual({ items: mockAvailabilityDefaults })
  })

  it("GET /api/availability?from&to", async () => {
    const fetchFn = mockFetch()
    await getAvailabilityRange("2026-10-26", "2026-11-01")
    expect(fetchFn.mock.calls[0][0]).toBe(
      "/api/availability?from=2026-10-26&to=2026-11-01"
    )
  })

  it("PUT /api/availability/{date}/{slotId}", async () => {
    const fetchFn = mockFetch(200, { cell: {} })
    await setAvailabilityOverride("2026-10-27", 2, {
      mode: "none",
      remove_plate: true,
    })
    const [url, init] = fetchFn.mock.calls[0]
    expect(url).toBe("/api/availability/2026-10-27/2")
    expect(init.method).toBe("PUT")
    expect(JSON.parse(init.body)).toEqual({ mode: "none", remove_plate: true })
  })

  it("DELETE /api/availability/{date}/{slotId}", async () => {
    const fetchFn = mockFetch(204)
    await expect(
      clearAvailabilityOverride("2026-10-27", 2)
    ).resolves.toBeUndefined()
    const [url, init] = fetchFn.mock.calls[0]
    expect(url).toBe("/api/availability/2026-10-27/2")
    expect(init.method).toBe("DELETE")
  })
})
