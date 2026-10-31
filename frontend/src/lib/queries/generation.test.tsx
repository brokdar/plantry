import { QueryClient } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { mockRun } from "@/test/fixtures"
import { createHookWrapper } from "@/test/render"

vi.mock("@/lib/api/generation", () => ({
  generatePlan: vi.fn(),
  regeneratePlan: vi.fn(),
  getPlanRun: vi.fn(),
}))

const navigateMock = vi.hoisted(() => vi.fn())
vi.mock("@tanstack/react-router", () => ({ useNavigate: () => navigateMock }))

const toastErrorMock = vi.hoisted(() => vi.fn())
vi.mock("@/lib/toast", () => ({
  toast: Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn() }),
  toastError: toastErrorMock,
}))

import { ApiError } from "@/lib/api/client"
import { generatePlan, getPlanRun, regeneratePlan } from "@/lib/api/generation"

import { RUN_POLL_MS, useGenerate, useRegenerate, useRun } from "./generation"
import { planKeys, shoppingKeys, todayKeys } from "./keys"

beforeEach(() => vi.clearAllMocks())

describe("useGenerate / useRegenerate", () => {
  it("useGenerate returns the run id and opens /plan?start&run", async () => {
    vi.mocked(generatePlan).mockResolvedValue({ run_id: 12 })
    const { result } = renderHook(() => useGenerate(), {
      wrapper: createHookWrapper(),
    })
    let res: unknown
    await act(async () => {
      res = await result.current.mutateAsync({ start: "2026-10-31" })
    })
    expect(res).toEqual({ run_id: 12 })
    expect(generatePlan).toHaveBeenCalledWith({ start: "2026-10-31" })
    expect(navigateMock).toHaveBeenCalledWith({
      to: "/plan",
      search: { start: "2026-10-31", run: 12 },
    })
  })

  it("useRegenerate returns the run id and opens /plan?start&run", async () => {
    vi.mocked(regeneratePlan).mockResolvedValue({ run_id: 13 })
    const { result } = renderHook(() => useRegenerate(), {
      wrapper: createHookWrapper(),
    })
    const input = {
      scope: { kind: "plate" as const, plate_id: 2 },
      start: "2026-10-31",
    }
    let res: unknown
    await act(async () => {
      res = await result.current.mutateAsync(input)
    })
    expect(res).toEqual({ run_id: 13 })
    expect(regeneratePlan).toHaveBeenCalledWith(input)
    expect(navigateMock).toHaveBeenCalledWith({
      to: "/plan",
      search: { start: "2026-10-31", run: 13 },
    })
  })

  it("a refused start toasts the error and stays put", async () => {
    vi.mocked(generatePlan).mockRejectedValue(
      new ApiError(409, "error.plan.generation_running")
    )
    const { result } = renderHook(() => useGenerate(), {
      wrapper: createHookWrapper(),
    })
    await act(async () => {
      await result.current.mutateAsync({ start: "2026-10-31" }).catch(() => {})
    })
    expect(toastErrorMock).toHaveBeenCalledTimes(1)
    expect(navigateMock).not.toHaveBeenCalled()
  })
})

describe("useRun (F1-AC2)", () => {
  beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }))
  afterEach(() => vi.useRealTimers())

  function setup(id: number | undefined) {
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const spy = vi.spyOn(qc, "invalidateQueries")
    const hook = renderHook(() => useRun(id), {
      wrapper: createHookWrapper(qc),
    })
    const invalidated = () => spy.mock.calls.map((c) => c[0]?.queryKey)
    return { ...hook, qc, invalidated }
  }

  const PLAN_VIEWS = [planKeys.all, todayKeys.all, shoppingKeys.all]

  it.each(["ok", "error", "partial"] as const)(
    "polls while running, stops on %s and invalidates the plan views once",
    async (status) => {
      vi.mocked(getPlanRun)
        .mockResolvedValueOnce(mockRun("running", "resolving"))
        .mockResolvedValueOnce(mockRun("running", "choosing"))
        .mockResolvedValue(mockRun(status))
      const { result, invalidated } = setup(12)

      await waitFor(() => expect(result.current.data?.stage).toBe("resolving"))
      expect(invalidated()).toEqual([])

      await act(() => vi.advanceTimersByTimeAsync(RUN_POLL_MS))
      await waitFor(() => expect(result.current.data?.stage).toBe("choosing"))
      expect(invalidated()).toEqual([])

      await act(() => vi.advanceTimersByTimeAsync(RUN_POLL_MS))
      await waitFor(() => expect(result.current.data?.status).toBe(status))
      expect(getPlanRun).toHaveBeenCalledTimes(3)
      expect(getPlanRun).toHaveBeenCalledWith(12)
      expect(invalidated()).toEqual(PLAN_VIEWS)

      // Finished: no more polling, no second invalidation.
      await act(() => vi.advanceTimersByTimeAsync(RUN_POLL_MS * 4))
      expect(getPlanRun).toHaveBeenCalledTimes(3)
      expect(invalidated()).toEqual(PLAN_VIEWS)
    }
  )

  it("a refetch of a finished run does not invalidate again", async () => {
    vi.mocked(getPlanRun).mockResolvedValue(mockRun("ok"))
    const { result, invalidated } = setup(12)
    await waitFor(() => expect(result.current.data?.status).toBe("ok"))
    expect(invalidated()).toEqual(PLAN_VIEWS)

    await act(async () => {
      await result.current.refetch()
    })
    expect(getPlanRun).toHaveBeenCalledTimes(2)
    expect(invalidated()).toEqual(PLAN_VIEWS)
  })

  it("without a run id it fetches nothing", async () => {
    const { result } = setup(undefined)
    await act(() => vi.advanceTimersByTimeAsync(RUN_POLL_MS * 2))
    expect(result.current.fetchStatus).toBe("idle")
    expect(getPlanRun).not.toHaveBeenCalled()
  })
})
