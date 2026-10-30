import { QueryClient } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import {
  mockAvailabilityCells,
  mockAvailabilityDefaults,
} from "@/test/fixtures"
import { createHookWrapper } from "@/test/render"

vi.mock("@/lib/api/availability", () => ({
  getAvailabilityDefaults: vi.fn(),
  setAvailabilityDefaults: vi.fn(),
  getAvailabilityRange: vi.fn(),
  setAvailabilityOverride: vi.fn(),
  clearAvailabilityOverride: vi.fn(),
}))

vi.mock("@/lib/toast", () => ({ toastError: vi.fn() }))

import {
  clearAvailabilityOverride,
  getAvailabilityDefaults,
  getAvailabilityRange,
  setAvailabilityDefaults,
  setAvailabilityOverride,
  type AvailabilityDefault,
} from "@/lib/api/availability"
import { ApiError } from "@/lib/api/client"
import { toastError } from "@/lib/toast"

import {
  useAvailabilityDefaults,
  useAvailabilityRange,
  useClearAvailabilityOverride,
  useSetAvailabilityDefaults,
  useSetAvailabilityOverride,
} from "./availability"
import { availabilityKeys, plateKeys } from "./keys"

function setup() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const invalidate = vi.spyOn(qc, "invalidateQueries")
  return { qc, invalidate, wrapper: createHookWrapper(qc) }
}

const invalidatedKeys = (spy: ReturnType<typeof setup>["invalidate"]) =>
  spy.mock.calls.map((c) => c[0]?.queryKey)

describe("availability queries", () => {
  beforeEach(() => vi.clearAllMocks())

  it("useAvailabilityDefaults loads the defaults grid", async () => {
    vi.mocked(getAvailabilityDefaults).mockResolvedValueOnce({
      items: mockAvailabilityDefaults,
    })
    const { wrapper } = setup()
    const { result } = renderHook(() => useAvailabilityDefaults(), { wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.items).toEqual(mockAvailabilityDefaults)
  })

  it("useAvailabilityRange calls the range endpoint with from/to", async () => {
    vi.mocked(getAvailabilityRange).mockResolvedValueOnce({
      items: mockAvailabilityCells,
    })
    const { wrapper } = setup()
    const { result } = renderHook(
      () => useAvailabilityRange("2026-10-26", "2026-11-01"),
      { wrapper }
    )
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(getAvailabilityRange).toHaveBeenCalledWith(
      "2026-10-26",
      "2026-11-01"
    )
  })

  it("useAvailabilityRange stays idle without a window", () => {
    const { wrapper } = setup()
    renderHook(() => useAvailabilityRange("", ""), { wrapper })
    expect(getAvailabilityRange).not.toHaveBeenCalled()
  })

  it("useSetAvailabilityDefaults PUTs the grid, caches it and invalidates only ranges", async () => {
    vi.mocked(setAvailabilityDefaults).mockResolvedValueOnce({
      items: mockAvailabilityDefaults,
    })
    const { qc, wrapper, invalidate } = setup()
    const { result } = renderHook(() => useSetAvailabilityDefaults(), {
      wrapper,
    })
    await act(() => result.current.mutateAsync(mockAvailabilityDefaults))
    expect(setAvailabilityDefaults).toHaveBeenCalledWith(
      mockAvailabilityDefaults
    )
    expect(qc.getQueryData(availabilityKeys.defaults())).toEqual({
      items: mockAvailabilityDefaults,
    })
    expect(invalidatedKeys(invalidate)).toEqual([availabilityKeys.ranges()])
  })

  it("useSetAvailabilityDefaults applies quick edits at once and the last PUT holds all of them", async () => {
    const plan = (weekday: number): AvailabilityDefault => ({
      weekday,
      slot_id: 1,
      mode: "plan" as const,
      preset_id: null,
    })
    const stored = [0, 1, 2].map(plan)
    const sent: AvailabilityDefault[][] = []
    vi.mocked(setAvailabilityDefaults).mockImplementation(async (items) => {
      sent.push(items)
      return { items }
    })
    const { qc, wrapper } = setup()
    qc.setQueryData(availabilityKeys.defaults(), { items: stored })
    const { result } = renderHook(() => useSetAvailabilityDefaults(), {
      wrapper,
    })

    // Three edits in one tick, each against the same (stale) render.
    const none = (weekday: number) => (current: AvailabilityDefault[]) =>
      current.map((c) =>
        c.weekday === weekday ? { ...c, mode: "none" as const } : c
      )
    act(() => {
      result.current.mutate(none(0))
      result.current.mutate(none(1))
      result.current.mutate(none(2))
    })
    const allNone = [0, 1, 2].map((weekday) => ({
      ...plan(weekday),
      mode: "none",
    }))
    // Optimistic: the cache shows all three before any response.
    expect(qc.getQueryData(availabilityKeys.defaults())).toEqual({
      items: allNone,
    })

    await waitFor(() => expect(qc.isMutating()).toBe(0))
    // Serialized PUTs; the last one carries every change.
    expect(sent.at(-1)).toEqual(allNone)
    expect(qc.getQueryData(availabilityKeys.defaults())).toEqual({
      items: allNone,
    })
  })

  it("useSetAvailabilityDefaults refetches the stored grid when a save fails", async () => {
    vi.mocked(setAvailabilityDefaults).mockRejectedValueOnce(
      new ApiError(500, "error.server")
    )
    const { wrapper, invalidate } = setup()
    const { result } = renderHook(() => useSetAvailabilityDefaults(), {
      wrapper,
    })
    await act(() =>
      result.current.mutateAsync(mockAvailabilityDefaults).catch(() => {})
    )
    expect(toastError).toHaveBeenCalled()
    expect(invalidatedKeys(invalidate)).toContainEqual(
      availabilityKeys.defaults()
    )
  })

  it("useSetAvailabilityOverride invalidates ranges but not plates when nothing was removed", async () => {
    vi.mocked(setAvailabilityOverride).mockResolvedValueOnce({
      cell: mockAvailabilityCells[0],
      removed: null,
    })
    const { wrapper, invalidate } = setup()
    const { result } = renderHook(() => useSetAvailabilityOverride(), {
      wrapper,
    })
    await act(() =>
      result.current.mutateAsync({
        date: "2026-10-26",
        slotId: 1,
        input: { mode: "none" },
      })
    )
    expect(setAvailabilityOverride).toHaveBeenCalledWith("2026-10-26", 1, {
      mode: "none",
    })
    const keys = invalidatedKeys(invalidate)
    expect(keys).toContainEqual(availabilityKeys.ranges())
    expect(keys).not.toContainEqual(plateKeys.all)
  })

  it("useSetAvailabilityOverride invalidates plates when a plate was removed", async () => {
    vi.mocked(setAvailabilityOverride).mockResolvedValueOnce({
      cell: mockAvailabilityCells[0],
      removed: { plates: [], sessions: [] },
    })
    const { wrapper, invalidate } = setup()
    const { result } = renderHook(() => useSetAvailabilityOverride(), {
      wrapper,
    })
    await act(() =>
      result.current.mutateAsync({
        date: "2026-04-13",
        slotId: 1,
        input: { mode: "none", remove_plate: true },
      })
    )
    expect(invalidatedKeys(invalidate)).toContainEqual(plateKeys.all)
  })

  it("useSetAvailabilityOverride toasts the API error key", async () => {
    const err = new ApiError(409, "error.availability.cell_has_plate")
    vi.mocked(setAvailabilityOverride).mockRejectedValueOnce(err)
    const { wrapper } = setup()
    const { result } = renderHook(() => useSetAvailabilityOverride(), {
      wrapper,
    })
    await act(async () => {
      await result.current
        .mutateAsync({ date: "2026-10-26", slotId: 1, input: { mode: "none" } })
        .catch(() => {})
    })
    expect(toastError).toHaveBeenCalledWith(err, expect.any(Function))
  })

  it("useClearAvailabilityOverride DELETEs and invalidates ranges", async () => {
    vi.mocked(clearAvailabilityOverride).mockResolvedValueOnce(undefined)
    const { wrapper, invalidate } = setup()
    const { result } = renderHook(() => useClearAvailabilityOverride(), {
      wrapper,
    })
    await act(() =>
      result.current.mutateAsync({ date: "2026-10-26", slotId: 3 })
    )
    expect(clearAvailabilityOverride).toHaveBeenCalledWith("2026-10-26", 3)
    expect(invalidatedKeys(invalidate)).toContainEqual(
      availabilityKeys.ranges()
    )
  })
})
