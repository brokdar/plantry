import { act, renderHook, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { createHookWrapper } from "@/test/render"

vi.mock("@/lib/api/slots", () => ({
  listTimeSlots: vi.fn(),
  createTimeSlot: vi.fn(),
  updateTimeSlot: vi.fn(),
  deleteTimeSlot: vi.fn(),
  getSlotBands: vi.fn(),
  setSlotBands: vi.fn(),
  applySlotBandPreset: vi.fn(),
}))
vi.mock("@/lib/toast", () => ({ toastError: vi.fn() }))

import { createTimeSlot, deleteTimeSlot, updateTimeSlot } from "@/lib/api/slots"
import { queryClient } from "@/lib/query-client"

import { slotKeys } from "./keys"
import {
  useCreateTimeSlot,
  useDeleteTimeSlot,
  useUpdateTimeSlot,
} from "./slots"

// SB1: a slot change gives the slots new bands on the server, so the band
// editor's query must refetch with the slot list.
describe("slot mutations refresh the bands", () => {
  beforeEach(() => vi.clearAllMocks())

  /** Whether an invalidation reaches slotKeys.bands() (equal or a parent key). */
  const reachesBands = (spy: { mock: { calls: unknown[][] } }) =>
    spy.mock.calls.some((c) => {
      const key = (c[0] as { queryKey?: readonly unknown[] })?.queryKey ?? []
      return key.every((part, i) => slotKeys.bands()[i] === part)
    })

  async function run<V>(
    useHook: () => { mutate: (v: V) => void; isSuccess: boolean },
    arg: V
  ) {
    const spy = vi.spyOn(queryClient, "invalidateQueries")
    const { result } = renderHook(useHook, {
      wrapper: createHookWrapper(queryClient),
    })
    await act(async () => {
      result.current.mutate(arg)
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const ok = reachesBands(spy)
    spy.mockRestore()
    return ok
  }

  it("create", async () => {
    vi.mocked(createTimeSlot).mockResolvedValueOnce({} as never)
    expect(await run(useCreateTimeSlot, {} as never)).toBe(true)
  })
  it("update", async () => {
    vi.mocked(updateTimeSlot).mockResolvedValueOnce({} as never)
    expect(await run(useUpdateTimeSlot, { id: 1, data: {} as never })).toBe(
      true
    )
  })
  it("delete", async () => {
    vi.mocked(deleteTimeSlot).mockResolvedValueOnce(undefined as never)
    expect(await run(useDeleteTimeSlot, 1)).toBe(true)
  })
})
