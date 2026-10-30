import { act, renderHook, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ApiError } from "@/lib/api/client"
import { mockPlan, mockRemovedBatch, mockTodayByVerdict } from "@/test/fixtures"
import { createHookWrapper, invalidatedBy } from "@/test/render"

vi.mock("@/lib/api/plan", () => ({
  getPlan: vi.fn(),
  clearPlan: vi.fn(),
  restorePlan: vi.fn(),
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

import { clearPlan, getPlan, restorePlan } from "@/lib/api/plan"
import { getToday } from "@/lib/api/today"

import { planKeys, shoppingKeys, todayKeys } from "./keys"
import {
  useClearWindow,
  usePlan,
  useRestorePlan,
  useUndoableRemoval,
} from "./plan"
import { useToday } from "./today"

const PLAN_VIEWS = [planKeys.all, todayKeys.all, shoppingKeys.all]

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

  it("useToday(date) fetches that date", async () => {
    vi.mocked(getToday).mockResolvedValue(mockTodayByVerdict.nothing_to_cook)
    const { result } = renderHook(() => useToday("2026-11-02"), {
      wrapper: createHookWrapper(),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(getToday).toHaveBeenCalledWith("2026-11-02")
    expect(result.current.data?.verdict.kind).toBe("nothing_to_cook")
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
