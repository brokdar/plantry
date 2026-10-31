import { act, renderHook } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { useTodayISO } from "./useTodayISO"

function setVisibility(state: DocumentVisibilityState) {
  Object.defineProperty(document, "visibilityState", {
    value: state,
    configurable: true,
  })
  document.dispatchEvent(new Event("visibilitychange"))
}

describe("useTodayISO", () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date(2026, 10, 2, 23, 59, 0))
  })
  afterEach(() => {
    vi.useRealTimers()
    delete (document as { visibilityState?: unknown }).visibilityState
  })

  it("rolls over at local midnight", () => {
    const { result } = renderHook(() => useTodayISO())
    expect(result.current).toBe("2026-11-02")
    act(() => vi.advanceTimersByTime(60_000))
    expect(result.current).toBe("2026-11-03")
    act(() => vi.advanceTimersByTime(24 * 3600_000))
    expect(result.current).toBe("2026-11-04")
  })

  it("catches up when the page becomes visible again", () => {
    const { result } = renderHook(() => useTodayISO())
    // A hidden tab may not fire its timers: jump the clock without them.
    vi.setSystemTime(new Date(2026, 10, 5, 8, 0, 0))
    expect(result.current).toBe("2026-11-02")
    act(() => setVisibility("visible"))
    expect(result.current).toBe("2026-11-05")
  })
})
