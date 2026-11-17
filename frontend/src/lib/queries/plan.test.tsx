import { QueryClient } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ApiError } from "@/lib/api/client"
import { revertPlan } from "@/lib/api/meal"
import {
  mockPlan,
  mockRemovedBatch,
  mockReplanPreview,
  mockTodayByVerdict,
} from "@/test/fixtures"
import { createHookWrapper, invalidatedBy } from "@/test/render"

vi.mock("@/lib/api/plan", () => ({
  getPlan: vi.fn(),
  clearPlan: vi.fn(),
  restorePlan: vi.fn(),
  acceptPlan: vi.fn(),
  unacceptPlan: vi.fn(),
  discardPlan: vi.fn(),
  getReplanPreview: vi.fn(),
  replan: vi.fn(),
}))
vi.mock("@/lib/api/meal", () => ({
  revertPlan: vi.fn(),
}))
vi.mock("@/lib/api/plates", () => ({
  confirmPlate: vi.fn(),
  unlockPlate: vi.fn(),
}))
vi.mock("@/lib/api/sessions", () => ({
  confirmSession: vi.fn(),
  unlockSession: vi.fn(),
}))
const navigateMock = vi.hoisted(() => vi.fn())
vi.mock("@tanstack/react-router", () => ({ useNavigate: () => navigateMock }))
vi.mock("@/lib/api/availability", () => ({
  getAvailabilityRange: vi.fn(),
  setAvailabilityOverride: vi.fn(),
  clearAvailabilityOverride: vi.fn(),
}))
vi.mock("@/lib/api/today", () => ({ getToday: vi.fn() }))

const toastMock = vi.hoisted(() =>
  Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn() })
)
const toastErrorMock = vi.hoisted(() => vi.fn())
vi.mock("@/lib/toast", () => ({
  toast: toastMock,
  toastError: toastErrorMock,
}))

import {
  clearAvailabilityOverride,
  getAvailabilityRange,
  setAvailabilityOverride,
} from "@/lib/api/availability"
import {
  acceptPlan,
  clearPlan,
  discardPlan,
  getPlan,
  getReplanPreview,
  replan,
  restorePlan,
  unacceptPlan,
} from "@/lib/api/plan"
import { confirmPlate, unlockPlate } from "@/lib/api/plates"
import { confirmSession, unlockSession } from "@/lib/api/sessions"
import { getToday } from "@/lib/api/today"

import {
  aiKeys,
  availabilityKeys,
  planKeys,
  shoppingKeys,
  todayKeys,
} from "./keys"
import {
  UNDO_MS,
  useAccept,
  useClearWindow,
  useConfirm,
  useDiscard,
  emptyUndo,
  mergeUndo,
  usePlan,
  usePlanDay,
  usePlanMeal,
  useRevertUndo,
  useReplan,
  useReplanPreview,
  useRestorePlan,
  useSetNoPlateNote,
  useUnaccept,
  useUndoableRemoval,
  useUnlock,
} from "./plan"
import { useToday } from "./today"

const PLAN_VIEWS = [planKeys.all, todayKeys.all, shoppingKeys.all]
// Accept and unaccept also refresh the assistant: change lines flip to blocked.
const WITH_ASSISTANT = [...PLAN_VIEWS, aiKeys.all]

interface UndoToastOptions {
  duration: number
  action: { label: string; onClick: () => void }
  onDismiss: () => void
  onAutoClose: () => void
}

function lastToast(): { message: string; opts: UndoToastOptions } {
  const call = toastMock.mock.calls.at(-1)!
  return { message: call[0] as string, opts: call[1] as UndoToastOptions }
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(restorePlan).mockResolvedValue({ plate_ids: [1, 2, 3] })
})

describe("read models", () => {
  it("usePlan(start) fetches that window", async () => {
    vi.mocked(getPlan).mockResolvedValue(mockPlan)
    const { result } = renderHook(() => usePlan("2026-10-31"), {
      wrapper: createHookWrapper(),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(getPlan).toHaveBeenCalledWith("2026-10-31")
    expect(result.current.data?.cells).toHaveLength(21)
  })

  it("usePlan() asks for the anchored window", async () => {
    vi.mocked(getPlan).mockResolvedValue(mockPlan)
    const { result } = renderHook(() => usePlan(), {
      wrapper: createHookWrapper(),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(getPlan).toHaveBeenCalledWith(undefined)
  })

  it("useToday(date, today) fetches that date", async () => {
    vi.mocked(getToday).mockResolvedValue(mockTodayByVerdict.nothing_to_cook)
    const { result } = renderHook(() => useToday("2026-11-02", "2026-11-01"), {
      wrapper: createHookWrapper(),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(getToday).toHaveBeenCalledWith("2026-11-02", "2026-11-01")
    expect(result.current.data?.verdict.kind).toBe("nothing_to_cook")
  })

  it("useToday refetches the same date when the real today moves on", async () => {
    vi.mocked(getToday).mockResolvedValue(mockTodayByVerdict.nothing_to_cook)
    const { result, rerender } = renderHook(
      ({ today }) => useToday("2026-11-02", today),
      { wrapper: createHookWrapper(), initialProps: { today: "2026-11-01" } }
    )
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    rerender({ today: "2026-11-02" })
    await waitFor(() =>
      expect(getToday).toHaveBeenLastCalledWith("2026-11-02", "2026-11-02")
    )
  })
})

describe("plan mutations invalidate the plan views (F1-AC2)", () => {
  it("useRestorePlan", async () => {
    expect(
      await invalidatedBy(() => useRestorePlan(), mockRemovedBatch.removed)
    ).toEqual(PLAN_VIEWS)
    expect(restorePlan).toHaveBeenCalledWith(mockRemovedBatch.removed)
  })

  it("useClearWindow, offering Undo", async () => {
    vi.mocked(clearPlan).mockResolvedValue(mockRemovedBatch)
    expect(
      await invalidatedBy(() => useClearWindow(), {
        from: "2026-10-31",
        to: "2026-11-06",
      })
    ).toEqual(PLAN_VIEWS)
    expect(clearPlan).toHaveBeenCalledWith("2026-10-31", "2026-11-06")
    expect(lastToast().message).toBe("Plan cleared")
  })

  it("a failed restore toasts the error", async () => {
    const err = new ApiError(409, "error.plan.cell_occupied")
    vi.mocked(restorePlan).mockRejectedValue(err)
    const { result } = renderHook(() => useRestorePlan(), {
      wrapper: createHookWrapper(),
    })
    act(() => result.current.mutate(mockRemovedBatch.removed))
    await waitFor(() => expect(toastErrorMock).toHaveBeenCalled())
    expect(toastErrorMock.mock.calls[0][0]).toBe(err)
  })
})

// F1-AC3
describe("useUndoableRemoval", () => {
  function offer() {
    const { result } = renderHook(() => useUndoableRemoval(), {
      wrapper: createHookWrapper(),
    })
    act(() => result.current(mockRemovedBatch, "batch_removed"))
    return lastToast()
  }

  it("shows a 6 s toast named after the removal", () => {
    const { message, opts } = offer()
    expect(message).toBe("Batch removed")
    expect(opts.duration).toBe(6000)
    expect(opts.action.label).toBe("Undo")
  })

  it("Undo restores the snapshot once", async () => {
    const { opts } = offer()
    act(() => {
      opts.action.onClick()
      opts.action.onClick()
    })
    await waitFor(() => expect(restorePlan).toHaveBeenCalledTimes(1))
    expect(restorePlan).toHaveBeenCalledWith(mockRemovedBatch.removed)
  })

  it.each(["onAutoClose", "onDismiss"] as const)(
    "does nothing once the toast closed (%s)",
    async (close) => {
      const { opts } = offer()
      act(() => {
        opts[close]()
        opts.action.onClick()
      })
      await Promise.resolve()
      expect(restorePlan).not.toHaveBeenCalled()
    }
  )

  it("confirms a successful Undo", async () => {
    const { opts } = offer()
    act(() => opts.action.onClick())
    await waitFor(() =>
      expect(toastMock.success).toHaveBeenCalledWith("Restored")
    )
    expect(toastErrorMock).not.toHaveBeenCalled()
  })

  it.each(["error.plan.cell_occupied", "error.plate.cell_unavailable"])(
    "names the conflict when the cell changed since (%s)",
    async (key) => {
      vi.mocked(restorePlan).mockRejectedValue(new ApiError(409, key))
      const { opts } = offer()
      act(() => opts.action.onClick())
      await waitFor(() =>
        expect(toastMock.error).toHaveBeenCalledWith(
          "Can't undo: that meal was changed in the meantime."
        )
      )
      expect(toastErrorMock).not.toHaveBeenCalled()
      expect(toastMock.success).not.toHaveBeenCalled()
    }
  )

  it("offers nothing for an empty snapshot", () => {
    const { result } = renderHook(() => useUndoableRemoval(), {
      wrapper: createHookWrapper(),
    })
    act(() =>
      result.current({ removed: { plates: [], sessions: [] } }, "plan_cleared")
    )
    expect(toastMock).not.toHaveBeenCalled()
  })
})

describe("useRevertUndo", () => {
  const undo = { first_event_id: 4, last_event_id: 9, cells: [] }

  function offer(token = undo) {
    const { result } = renderHook(() => useRevertUndo(), {
      wrapper: createHookWrapper(),
    })
    act(() => result.current(token, "Moved", "Lasagne will now be cooked"))
    return toastMock.mock.calls.at(-1)
  }

  it("shows the undo toast with the also-changed line; Undo reverts once", async () => {
    vi.mocked(revertPlan).mockResolvedValue({ plate_ids: [1], session_ids: [] })
    offer()
    const { message, opts } = lastToast()
    expect(message).toBe("Moved")
    expect((opts as unknown as { description: string }).description).toBe(
      "Lasagne will now be cooked"
    )
    act(() => {
      opts.action.onClick()
      opts.action.onClick()
    })
    await waitFor(() => expect(revertPlan).toHaveBeenCalledTimes(1))
    expect(revertPlan).toHaveBeenCalledWith(undo)
    await waitFor(() =>
      expect(toastMock.success).toHaveBeenCalledWith("Restored")
    )
  })

  it("a stale token shows its message", async () => {
    vi.mocked(revertPlan).mockRejectedValue(
      new ApiError(409, "error.plan.change_stale")
    )
    offer()
    act(() => lastToast().opts.action.onClick())
    await waitFor(() => expect(toastErrorMock).toHaveBeenCalled())
    expect((toastErrorMock.mock.calls[0][0] as ApiError).messageKey).toBe(
      "error.plan.change_stale"
    )
  })

  it("offers nothing for an empty token", () => {
    expect(offer({ first_event_id: 0, last_event_id: 0, cells: [] })).toBe(
      undefined
    )
  })
})

describe("mergeUndo / emptyUndo", () => {
  it("spans the first to the last event and keeps every cell", () => {
    const cell = { date: "2026-11-02", slot_id: 1, before: null, after: null }
    expect(
      mergeUndo([
        { first_event_id: 3, last_event_id: 4, cells: [] },
        { first_event_id: 0, last_event_id: 0, cells: [cell] },
        { first_event_id: 7, last_event_id: 9, cells: [] },
      ])
    ).toEqual({ first_event_id: 3, last_event_id: 9, cells: [cell] })
    expect(emptyUndo({ first_event_id: 0, last_event_id: 0, cells: [] })).toBe(
      true
    )
    expect(emptyUndo(undefined)).toBe(true)
  })
})

describe("usePlanDay", () => {
  it("puts every No plate cell of the day back to Plan", async () => {
    vi.mocked(clearAvailabilityOverride).mockResolvedValue(undefined)
    vi.mocked(getAvailabilityRange).mockResolvedValue({ items: [] })
    vi.mocked(setAvailabilityOverride).mockResolvedValue({
      cell: {} as never,
      removed: null,
    })
    const cells = [
      {
        ...mockPlan.cells[0],
        mode: "none" as const,
        source: "default" as const,
      },
      { ...mockPlan.cells[1], mode: "plan" as const },
    ]
    const { result } = renderHook(() => usePlanDay(), {
      wrapper: createHookWrapper(),
    })
    await act(async () => {
      await result.current.mutateAsync(cells)
    })
    expect(setAvailabilityOverride).toHaveBeenCalledTimes(1)
    expect(setAvailabilityOverride).toHaveBeenCalledWith(
      cells[0].date,
      cells[0].slot_id,
      { mode: "plan" }
    )
  })
})

describe("No plate cell actions", () => {
  const none = {
    ...mockPlan.cells[0],
    mode: "none" as const,
    note: "Eating out",
  }
  const resolved = (mode: "plan" | "none") => ({
    items: [
      {
        date: none.date,
        slot_id: none.slot_id,
        mode,
        source: "default" as const,
        preset_id: null,
        preset_name: null,
        note: null,
      },
    ],
  })

  it("Plan this meal drops a date override when the default plans", async () => {
    vi.mocked(clearAvailabilityOverride).mockResolvedValue(undefined)
    vi.mocked(getAvailabilityRange).mockResolvedValue(resolved("plan"))
    const keys = await invalidatedBy(() => usePlanMeal(), {
      ...none,
      source: "override" as const,
    })
    expect(clearAvailabilityOverride).toHaveBeenCalledWith(
      none.date,
      none.slot_id
    )
    expect(setAvailabilityOverride).not.toHaveBeenCalled()
    expect(keys).toEqual([...PLAN_VIEWS, availabilityKeys.ranges()])
  })

  it("Plan this meal puts a plan override when the default is No plate", async () => {
    vi.mocked(clearAvailabilityOverride).mockResolvedValue(undefined)
    vi.mocked(getAvailabilityRange).mockResolvedValue(resolved("none"))
    await invalidatedBy(() => usePlanMeal(), {
      ...none,
      source: "override" as const,
    })
    expect(setAvailabilityOverride).toHaveBeenCalledWith(
      none.date,
      none.slot_id,
      { mode: "plan" }
    )
  })

  it("Plan this meal puts a plan override when the cell can't be resolved", async () => {
    vi.mocked(clearAvailabilityOverride).mockResolvedValue(undefined)
    vi.mocked(getAvailabilityRange).mockResolvedValue({ items: [] })
    await invalidatedBy(() => usePlanMeal(), {
      ...none,
      source: "override" as const,
    })
    expect(setAvailabilityOverride).toHaveBeenCalledWith(
      none.date,
      none.slot_id,
      { mode: "plan" }
    )
  })

  it("Plan this meal on a weekday default puts a plan override directly", async () => {
    await invalidatedBy(() => usePlanMeal(), none)
    expect(clearAvailabilityOverride).not.toHaveBeenCalled()
    expect(setAvailabilityOverride).toHaveBeenCalledWith(
      none.date,
      none.slot_id,
      { mode: "plan" }
    )
  })

  it("Edit note keeps the cell No plate", async () => {
    const keys = await invalidatedBy(() => useSetNoPlateNote(), {
      cell: none,
      note: "At grandma's",
    })
    expect(setAvailabilityOverride).toHaveBeenCalledWith(
      none.date,
      none.slot_id,
      { mode: "none", note: "At grandma's" }
    )
    expect(keys).toEqual([...PLAN_VIEWS, availabilityKeys.ranges()])
  })
})

// ── Phase 3: draft / confirmed, accept, discard, re-plan ──────────────

const WINDOW = { from: "2026-10-31", to: "2026-11-06" }
const ACCEPTED = { plate_ids: [1, 2, 3], session_ids: [7] }

describe("confirm and unlock act on the closure", () => {
  it.each([
    ["plate", confirmPlate, unlockPlate],
    ["session", confirmSession, unlockSession],
  ] as const)("%s target", async (kind, confirm, unlock) => {
    vi.mocked(confirm).mockResolvedValue(ACCEPTED)
    vi.mocked(unlock).mockResolvedValue(ACCEPTED)
    expect(await invalidatedBy(() => useConfirm(), { kind, id: 7 })).toEqual(
      PLAN_VIEWS
    )
    expect(confirm).toHaveBeenCalledWith(7)
    expect(await invalidatedBy(() => useUnlock(), { kind, id: 7 })).toEqual(
      PLAN_VIEWS
    )
    expect(unlock).toHaveBeenCalledWith(7)
  })

  it("F2-AC2: Confirm offers a 6 s Undo that unlocks the same target", async () => {
    vi.mocked(confirmPlate).mockResolvedValue(ACCEPTED)
    vi.mocked(unlockPlate).mockResolvedValue(ACCEPTED)
    await invalidatedBy(() => useConfirm(), { kind: "plate", id: 2 })
    const { message, opts } = lastToast()
    expect(message).toBe("3 plates confirmed.")
    expect(opts.duration).toBe(UNDO_MS)
    opts.action.onClick()
    await waitFor(() => expect(unlockPlate).toHaveBeenCalledWith(2))
  })
})

// Phase 3 F1-AC3
describe("useAccept", () => {
  async function accept() {
    vi.mocked(acceptPlan).mockResolvedValue(ACCEPTED)
    vi.mocked(unacceptPlan).mockResolvedValue(ACCEPTED)
    const keys = await invalidatedBy(() => useAccept(), WINDOW)
    return { keys, ...lastToast() }
  }

  it("accepts the window and offers Undo", async () => {
    const { keys, message, opts } = await accept()
    expect(acceptPlan).toHaveBeenCalledWith(WINDOW.from, WINDOW.to)
    expect(keys).toEqual(WITH_ASSISTANT)
    expect(message).toBe("Plan accepted. 3 plates confirmed.")
    expect(opts.duration).toBe(6000)
  })

  it("Undo unaccepts exactly the returned ids, once", async () => {
    const { opts } = await accept()
    act(() => {
      opts.action.onClick()
      opts.action.onClick()
    })
    await waitFor(() => expect(unacceptPlan).toHaveBeenCalledTimes(1))
    expect(vi.mocked(unacceptPlan).mock.calls[0][0]).toEqual(ACCEPTED)
    await waitFor(() =>
      expect(toastMock.success).toHaveBeenCalledWith("Restored")
    )
  })

  it("nothing to accept offers no Undo", async () => {
    vi.mocked(acceptPlan).mockResolvedValue({ plate_ids: [], session_ids: [] })
    await invalidatedBy(() => useAccept(), WINDOW)
    expect(toastMock).not.toHaveBeenCalled()
  })

  it("useUnaccept on its own invalidates the plan views", async () => {
    vi.mocked(unacceptPlan).mockResolvedValue(ACCEPTED)
    expect(await invalidatedBy(() => useUnaccept(), ACCEPTED)).toEqual(
      WITH_ASSISTANT
    )
  })
})

describe("useDiscard", () => {
  it("discards the drafts and Undo restores them", async () => {
    vi.mocked(discardPlan).mockResolvedValue(mockRemovedBatch)
    expect(await invalidatedBy(() => useDiscard(), WINDOW)).toEqual(PLAN_VIEWS)
    expect(discardPlan).toHaveBeenCalledWith(WINDOW.from, WINDOW.to)
    const { message, opts } = lastToast()
    expect(message).toBe("Draft discarded")
    act(() => opts.action.onClick())
    await waitFor(() =>
      expect(restorePlan).toHaveBeenCalledWith(mockRemovedBatch.removed)
    )
  })
})

describe("re-plan", () => {
  const input = {
    from: "2026-11-04",
    to: "2026-11-06",
    unlock_confirmed: true,
    spare: "freeze" as const,
    regenerate: true,
  }

  it("useReplanPreview fetches the range with the asked No-plate cells", async () => {
    vi.mocked(getReplanPreview).mockResolvedValue(mockReplanPreview)
    const cells = [{ date: "2026-11-05", slot_id: 3 }]
    const { result } = renderHook(
      () => useReplanPreview("2026-11-04", "2026-11-06", cells),
      { wrapper: createHookWrapper() }
    )
    await waitFor(() => expect(result.current.data).toEqual(mockReplanPreview))
    expect(getReplanPreview).toHaveBeenCalledWith(
      "2026-11-04",
      "2026-11-06",
      cells
    )
  })

  it("useReplanPreview waits while disabled", () => {
    renderHook(() => useReplanPreview("2026-11-04", "2026-11-06", [], false), {
      wrapper: createHookWrapper(),
    })
    expect(getReplanPreview).not.toHaveBeenCalled()
  })

  it("useReplan with a started run attaches the board to it", async () => {
    vi.mocked(replan).mockResolvedValue({
      ...mockRemovedBatch,
      run_id: 14,
      unassigned: 0,
    })
    expect(await invalidatedBy(() => useReplan(), input)).toEqual([
      availabilityKeys.ranges(),
      ...PLAN_VIEWS,
    ])
    expect(replan).toHaveBeenCalledWith(input)
    expect(navigateMock).toHaveBeenCalledTimes(1)
    const { to, search } = navigateMock.mock.calls[0][0]
    expect(to).toBe("/plan")
    expect(search({ start: "2026-10-31" })).toEqual({
      start: "2026-10-31",
      run: 14,
    })
  })

  it("useReplan without a run stays on the board", async () => {
    vi.mocked(replan).mockResolvedValue({
      ...mockRemovedBatch,
      run_id: null,
      unassigned: 0,
    })
    await invalidatedBy(() => useReplan(), { ...input, regenerate: false })
    expect(navigateMock).not.toHaveBeenCalled()
  })

  it("a failed refresh after the saved re-plan is no mutation error; the board still opens the run", async () => {
    vi.mocked(replan).mockResolvedValue({
      ...mockRemovedBatch,
      run_id: 14,
      unassigned: 0,
    })
    const qc = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    vi.spyOn(qc, "invalidateQueries").mockRejectedValue(new Error("offline"))
    const { result } = renderHook(() => useReplan(), {
      wrapper: createHookWrapper(qc),
    })
    await act(async () => {
      await result.current.mutateAsync(input)
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(toastErrorMock).not.toHaveBeenCalled()
    expect(navigateMock).toHaveBeenCalledTimes(1)
  })
})
