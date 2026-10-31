import { act, renderHook } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { Session } from "@/lib/api/sessions"
import { mockSession } from "@/test/fixtures"
import { createHookWrapper } from "@/test/render"

vi.mock("@/lib/api/sessions", () => ({ updateSession: vi.fn() }))
vi.mock("@/lib/toast", () => ({ toast: vi.fn(), toastError: vi.fn() }))

import { updateSession } from "@/lib/api/sessions"

import { PORTIONS_DEBOUNCE_MS, useSessionPortions } from "./useSessionPortions"

function deferred() {
  let resolve!: (s: Session) => void
  const promise = new Promise<Session>((r) => (resolve = r))
  return { promise, resolve }
}

const session: Session = { ...mockSession, frozen_portions: 2, note: "Spicy" }

function render() {
  return renderHook(() => useSessionPortions(session), {
    wrapper: createHookWrapper(),
  })
}

describe("useSessionPortions", () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    vi.mocked(updateSession).mockImplementation(async (_id, input) => ({
      ...session,
      ...input,
    }))
  })
  afterEach(() => vi.useRealTimers())

  it("shows the change at once and writes once after 400 ms of quiet", async () => {
    const { result } = render()
    expect(result.current.portions).toBe(6)

    act(() => result.current.change(7))
    act(() => result.current.change(8))
    expect(result.current.portions).toBe(8)
    await act(() => vi.advanceTimersByTimeAsync(PORTIONS_DEBOUNCE_MS - 1))
    expect(updateSession).not.toHaveBeenCalled()

    await act(() => vi.advanceTimersByTimeAsync(1))
    expect(updateSession).toHaveBeenCalledTimes(1)
    expect(updateSession).toHaveBeenCalledWith(7, {
      portions: 8,
      frozen_portions: 2,
      note: "Spicy",
    })
  })

  it("clamps frozen portions to the new total", async () => {
    const { result } = render()
    act(() => result.current.change(1))
    await act(() => vi.advanceTimersByTimeAsync(PORTIONS_DEBOUNCE_MS))
    expect(updateSession).toHaveBeenCalledWith(7, {
      portions: 1,
      frozen_portions: 1,
      note: "Spicy",
    })
  })

  it("flushes a pending edit on unmount", async () => {
    const { result, unmount } = render()
    act(() => result.current.change(9))
    unmount()
    await act(() => vi.advanceTimersByTimeAsync(0))
    expect(updateSession).toHaveBeenCalledTimes(1)
    expect(updateSession).toHaveBeenCalledWith(
      7,
      expect.objectContaining({ portions: 9 })
    )
  })

  it("writes nothing on unmount without an edit", async () => {
    const { unmount } = render()
    unmount()
    await act(() => vi.advanceTimersByTimeAsync(PORTIONS_DEBOUNCE_MS))
    expect(updateSession).not.toHaveBeenCalled()
  })

  it("keeps a newer edit shown while it still waits to be written", async () => {
    const first = deferred()
    vi.mocked(updateSession).mockReturnValueOnce(first.promise)
    const { result } = render()

    act(() => result.current.change(7))
    await act(() => vi.advanceTimersByTimeAsync(PORTIONS_DEBOUNCE_MS))
    act(() => result.current.change(8))
    // The write of 7 settles while 8 is still debouncing.
    await act(async () => first.resolve({ ...session, portions: 7 }))
    expect(result.current.portions).toBe(8)

    await act(() => vi.advanceTimersByTimeAsync(PORTIONS_DEBOUNCE_MS))
    expect(vi.mocked(updateSession).mock.calls[1][1].portions).toBe(8)
  })

  it("sends writes in order and keeps the newest value shown", async () => {
    const first = deferred()
    const second = deferred()
    vi.mocked(updateSession)
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise)
    const { result } = render()

    act(() => result.current.change(7))
    await act(() => vi.advanceTimersByTimeAsync(PORTIONS_DEBOUNCE_MS))
    act(() => result.current.change(8))
    await act(() => vi.advanceTimersByTimeAsync(PORTIONS_DEBOUNCE_MS))

    // The second write waits for the first (one mutation scope).
    expect(updateSession).toHaveBeenCalledTimes(1)
    await act(async () => first.resolve({ ...session, portions: 7 }))
    expect(result.current.portions).toBe(8)
    expect(updateSession).toHaveBeenCalledTimes(2)
    expect(vi.mocked(updateSession).mock.calls[1][1].portions).toBe(8)

    await act(async () => second.resolve({ ...session, portions: 8 }))
  })
})
