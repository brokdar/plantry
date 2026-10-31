import { act, renderHook, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { createHookWrapper, invalidatedBy } from "@/test/render"

vi.mock("@/lib/api/plates", () => ({ deletePlate: vi.fn() }))
vi.mock("@/lib/api/plan", () => ({ restorePlan: vi.fn() }))
vi.mock("@/lib/api/availability", () => ({
  clearAvailabilityOverride: vi.fn(),
}))
const toastMock = vi.hoisted(() => Object.assign(vi.fn(), { warning: vi.fn() }))
vi.mock("@/lib/toast", () => ({ toast: toastMock, toastError: vi.fn() }))

import { clearAvailabilityOverride } from "@/lib/api/availability"
import { restorePlan } from "@/lib/api/plan"
import { deletePlate } from "@/lib/api/plates"
import type { ApplyResult } from "@/lib/api/presets"

import { availabilityKeys, planKeys, shoppingKeys, todayKeys } from "./keys"
import { useApplyResultToasts, useUndoApply } from "./presets"

beforeEach(() => vi.clearAllMocks())

const empty = { plates: [], sessions: [] }

function result(overrides: Partial<ApplyResult> = {}): ApplyResult {
  return {
    created_plate_ids: [],
    skipped_occupied: [],
    skipped_unavailable: [],
    skipped_no_slot: [],
    removed: empty,
    created_overrides: [],
    ...overrides,
  }
}

// Undo of an apply / copy week (B2): delete the created plates, restore what
// overwrite removed, clear the created No plate overrides.
describe("useUndoApply", () => {
  it("deletes, then restores, then clears overrides", async () => {
    vi.mocked(deletePlate).mockResolvedValue({ removed: empty })
    vi.mocked(restorePlan).mockResolvedValue({ plate_ids: [9] })
    vi.mocked(clearAvailabilityOverride).mockResolvedValue(undefined)
    const removed = {
      plates: [
        {
          date: "2026-11-02",
          slot_id: 1,
          eaters: 2,
          note: null,
          components: [
            { food_id: 4, portions: 1, sort_order: 0, session: { ref: "s1" } },
          ],
        },
      ],
      sessions: [
        {
          ref: "s1",
          food_id: 4,
          cook_date: "2026-11-02",
          cook_slot_id: 1,
          portions: 2,
          frozen_portions: 0,
          cooked_at: null,
          note: null,
        },
      ],
    }

    const keys = await invalidatedBy(
      () => useUndoApply(),
      result({
        created_plate_ids: [5, 6],
        removed,
        created_overrides: [{ date: "2026-11-03", slot_id: 1 }],
      })
    )

    expect(vi.mocked(deletePlate).mock.calls).toEqual([[5], [6]])
    expect(restorePlan).toHaveBeenCalledWith(removed)
    expect(clearAvailabilityOverride).toHaveBeenCalledWith("2026-11-03", 1)
    const lastDelete = vi.mocked(deletePlate).mock.invocationCallOrder[1]!
    const restore = vi.mocked(restorePlan).mock.invocationCallOrder[0]!
    expect(lastDelete).toBeLessThan(restore)
    expect(keys).toEqual([
      availabilityKeys.ranges(),
      planKeys.all,
      todayKeys.all,
      shoppingKeys.all,
    ])
  })

  it("skips the restore when nothing was replaced", async () => {
    vi.mocked(deletePlate).mockResolvedValue({ removed: empty })
    await invalidatedBy(
      () => useUndoApply(),
      result({ created_plate_ids: [5] })
    )
    expect(deletePlate).toHaveBeenCalledWith(5)
    expect(restorePlan).not.toHaveBeenCalled()
  })
})

// F10-AC2: one toast flow for apply and copy week (plate sheet, palette,
// copy week dialog). Undo deletes what was created and restores what was
// replaced.
describe("useApplyResultToasts", () => {
  const replacedPlate = {
    date: "2026-11-02",
    slot_id: 1,
    eaters: 2,
    note: null,
    components: [],
  }
  const cell = (slot_id: number) => ({ date: "2026-11-02", slot_id })

  function show(r: ApplyResult) {
    const { result: hook } = renderHook(() => useApplyResultToasts(), {
      wrapper: createHookWrapper(),
    })
    act(() => hook.current(r))
  }

  it("names the created plates and offers a 6 s Undo that deletes them once", async () => {
    vi.mocked(deletePlate).mockResolvedValue({ removed: empty })
    show(result({ created_plate_ids: [5, 6] }))
    expect(toastMock).toHaveBeenCalledTimes(1)
    const [message, opts] = toastMock.mock.calls[0]!
    expect(message).toBe("Added 2 plates.")
    expect(opts.duration).toBe(6000)
    act(() => {
      opts.action.onClick()
      opts.action.onClick()
    })
    await waitFor(() => expect(deletePlate).toHaveBeenCalledTimes(2))
    expect(restorePlan).not.toHaveBeenCalled()
  })

  it("Undo after overwrite restores the replaced plates", async () => {
    vi.mocked(deletePlate).mockResolvedValue({ removed: empty })
    vi.mocked(restorePlan).mockResolvedValue({ plate_ids: [9] })
    const removed = { plates: [replacedPlate], sessions: [] }
    show(result({ created_plate_ids: [5, 6], removed }))
    const [message, opts] = toastMock.mock.calls[0]!
    expect(message).toBe("Added 1 plate. Replaced 1 plate.")
    act(() => opts.action.onClick())
    await waitFor(() => expect(restorePlan).toHaveBeenCalledWith(removed))
  })

  it("warns about skipped cells after the result", () => {
    show(
      result({
        created_plate_ids: [5],
        skipped_occupied: [cell(2)],
        skipped_unavailable: [cell(3), cell(4)],
        skipped_no_slot: [cell(5)],
      })
    )
    expect(toastMock.warning.mock.calls.map((c) => c[0])).toEqual([
      "Skipped 1 meal that already had a plate.",
      "Left out 2 No plate meals.",
      "1 plate could not find its meal slot.",
    ])
    expect(toastMock.mock.invocationCallOrder[0]).toBeLessThan(
      toastMock.warning.mock.invocationCallOrder[0]!
    )
  })

  it("offers nothing when nothing was created", () => {
    show(result({ skipped_occupied: [cell(2)] }))
    expect(toastMock).not.toHaveBeenCalled()
    expect(toastMock.warning).toHaveBeenCalledTimes(1)
  })
})
