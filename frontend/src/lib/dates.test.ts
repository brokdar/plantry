import { afterEach, describe, expect, it, vi } from "vitest"

import { shiftYMD, todayISO } from "./dates"

describe("todayISO", () => {
  afterEach(() => vi.useRealTimers())

  it("is the local calendar date", () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date(2026, 9, 31, 23, 59))
    expect(todayISO()).toBe("2026-10-31")
  })
})

describe("shiftYMD", () => {
  it.each([
    ["2026-10-31", 7, "2026-11-07"],
    ["2026-11-06", 1, "2026-11-07"],
    ["2026-01-01", -1, "2025-12-31"],
    // Across the end of daylight saving time in Europe (25 Oct 2026).
    ["2026-10-24", 2, "2026-10-26"],
    ["2028-02-28", 1, "2028-02-29"],
  ])("%s shifted by %i days is %s", (from, days, want) => {
    expect(shiftYMD(from, days)).toBe(want)
  })
})
