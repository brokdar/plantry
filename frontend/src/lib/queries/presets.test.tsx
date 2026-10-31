import { beforeEach, describe, expect, it, vi } from "vitest"

import { invalidatedBy } from "@/test/render"

vi.mock("@/lib/api/plates", () => ({ deletePlate: vi.fn() }))
vi.mock("@/lib/api/plan", () => ({ restorePlan: vi.fn() }))
vi.mock("@/lib/api/availability", () => ({
  clearAvailabilityOverride: vi.fn(),
}))
vi.mock("@/lib/toast", () => ({ toast: vi.fn(), toastError: vi.fn() }))

import { clearAvailabilityOverride } from "@/lib/api/availability"
import { restorePlan } from "@/lib/api/plan"
import { deletePlate } from "@/lib/api/plates"
import type { ApplyResult } from "@/lib/api/presets"

import {
  availabilityKeys,
  planKeys,
  plateKeys,
  shoppingKeys,
  todayKeys,
} from "./keys"
import { useUndoApply } from "./presets"

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
      plateKeys.all,
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
