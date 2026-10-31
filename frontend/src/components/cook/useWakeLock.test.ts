import { act, renderHook, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { useWakeLock } from "./useWakeLock"

function setWakeLock(value: unknown) {
  Object.defineProperty(navigator, "wakeLock", {
    value,
    configurable: true,
  })
}

describe("useWakeLock", () => {
  afterEach(() => {
    // jsdom has no wake lock; leave it that way for other tests.
    delete (navigator as { wakeLock?: unknown }).wakeLock
  })

  it("requests the screen lock on mount and releases it on unmount", async () => {
    const release = vi.fn().mockResolvedValue(undefined)
    const request = vi.fn().mockResolvedValue({ released: false, release })
    setWakeLock({ request })

    const { unmount } = renderHook(() => useWakeLock())
    expect(request).toHaveBeenCalledWith("screen")
    // Let the granted lock settle.
    await act(async () => {})

    unmount()
    expect(release).toHaveBeenCalledTimes(1)
  })

  it("ignores a refusal", async () => {
    const request = vi.fn().mockRejectedValue(new DOMException("denied"))
    setWakeLock({ request })

    const { unmount } = renderHook(() => useWakeLock())
    await waitFor(() => expect(request).toHaveBeenCalled())
    expect(() => unmount()).not.toThrow()
  })

  it("does nothing without the API", () => {
    expect("wakeLock" in navigator).toBe(false)
    const { unmount } = renderHook(() => useWakeLock())
    expect(() => unmount()).not.toThrow()
  })
})
