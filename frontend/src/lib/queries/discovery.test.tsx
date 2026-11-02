import { QueryClient } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { mockDiscoveryRun, mockSuggestion } from "@/test/fixtures"
import { createHookWrapper, invalidatedBy } from "@/test/render"

vi.mock("@/lib/api/discovery", () => ({
  discover: vi.fn(),
  getDiscoveryRun: vi.fn(),
  getDailySuggestion: vi.fn(),
  anotherSuggestion: vi.fn(),
  listSuggestions: vi.fn(),
  acceptSuggestion: vi.fn(),
  rejectSuggestion: vi.fn(),
}))

vi.mock("@/lib/api/foods", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/foods")>()),
  createFood: vi.fn(),
}))

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn(), warning: vi.fn() },
}))

import { toast } from "sonner"

import { createFood } from "@/lib/api/foods"

import {
  acceptSuggestion,
  anotherSuggestion,
  discover,
  getDailySuggestion,
  getDiscoveryRun,
  listSuggestions,
  rejectSuggestion,
} from "@/lib/api/discovery"

import {
  DISCOVERY_POLL_MS,
  useAcceptSuggestion,
  useAnotherSuggestion,
  useDailySuggestion,
  useDiscover,
  useDiscoveryRun,
  useRejectSuggestion,
  useSuggestions,
} from "./discovery"
import { discoveryKeys, foodKeys, insightKeys } from "./keys"

beforeEach(() => {
  vi.clearAllMocks()
  vi.useRealTimers()
})

const DATE = "2026-11-02"
const suggestion = mockSuggestion()

describe("useDailySuggestion (F1-AC2)", () => {
  it("200: returns the suggestion and fetches once", async () => {
    vi.mocked(getDailySuggestion).mockResolvedValue({
      kind: "suggestion",
      suggestion,
    })
    const { result } = renderHook(() => useDailySuggestion(DATE, true), {
      wrapper: createHookWrapper(),
    })
    await waitFor(() =>
      expect(result.current.data).toEqual({ kind: "suggestion", suggestion })
    )
    expect(getDailySuggestion).toHaveBeenCalledTimes(1)
    expect(getDailySuggestion).toHaveBeenCalledWith(DATE)
    expect(getDiscoveryRun).not.toHaveBeenCalled()
  })

  it("204: is silent, kind none, no polling", async () => {
    vi.mocked(getDailySuggestion).mockResolvedValue({ kind: "none" })
    const { result } = renderHook(() => useDailySuggestion(DATE, true), {
      wrapper: createHookWrapper(),
    })
    await waitFor(() => expect(result.current.data).toEqual({ kind: "none" }))
    expect(getDiscoveryRun).not.toHaveBeenCalled()
  })

  it("a run that cannot be read stops polling and reads as none (F2)", async () => {
    vi.mocked(getDailySuggestion).mockResolvedValue({
      kind: "running",
      runId: 21,
    })
    vi.mocked(getDiscoveryRun)
      .mockResolvedValueOnce(mockDiscoveryRun("running"))
      .mockRejectedValue(new Error("404"))
    const { result } = renderHook(() => useDailySuggestion(DATE, true), {
      wrapper: createHookWrapper(),
    })
    await waitFor(() => expect(result.current.data?.kind).toBe("running"))
    await waitFor(() => expect(result.current.data).toEqual({ kind: "none" }), {
      timeout: 4000,
    })
    const calls = vi.mocked(getDiscoveryRun).mock.calls.length
    await new Promise((r) => setTimeout(r, DISCOVERY_POLL_MS * 2.5))
    expect(vi.mocked(getDiscoveryRun).mock.calls.length).toBe(calls)
  }, 15000)

  it("is idle when disabled", () => {
    renderHook(() => useDailySuggestion(DATE, false), {
      wrapper: createHookWrapper(),
    })
    expect(getDailySuggestion).not.toHaveBeenCalled()
  })

  it("202: polls the run every second while running, then refetches once", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    vi.mocked(getDailySuggestion)
      .mockResolvedValueOnce({ kind: "running", runId: 21 })
      .mockResolvedValueOnce({ kind: "suggestion", suggestion })
    vi.mocked(getDiscoveryRun)
      .mockResolvedValueOnce(mockDiscoveryRun("running"))
      .mockResolvedValueOnce(mockDiscoveryRun("running"))
      .mockResolvedValue(mockDiscoveryRun("ok"))
    const { result } = renderHook(() => useDailySuggestion(DATE, true), {
      wrapper: createHookWrapper(),
    })
    await waitFor(() => expect(getDiscoveryRun).toHaveBeenCalledTimes(1))
    expect(getDailySuggestion).toHaveBeenCalledTimes(1)
    await act(() => vi.advanceTimersByTimeAsync(DISCOVERY_POLL_MS))
    await waitFor(() => expect(getDiscoveryRun).toHaveBeenCalledTimes(2))
    await act(() => vi.advanceTimersByTimeAsync(DISCOVERY_POLL_MS))
    await waitFor(() =>
      expect(result.current.data).toEqual({ kind: "suggestion", suggestion })
    )
    expect(getDailySuggestion).toHaveBeenCalledTimes(2)
    // Done: no further polling.
    await act(() => vi.advanceTimersByTimeAsync(DISCOVERY_POLL_MS * 3))
    expect(getDiscoveryRun).toHaveBeenCalledTimes(3)
    expect(getDailySuggestion).toHaveBeenCalledTimes(2)
  })

  it("202 whose run failed: refetch ends in none (silent)", async () => {
    vi.mocked(getDailySuggestion)
      .mockResolvedValueOnce({ kind: "running", runId: 21 })
      .mockResolvedValueOnce({ kind: "none" })
    vi.mocked(getDiscoveryRun).mockResolvedValue(mockDiscoveryRun("error"))
    const { result } = renderHook(() => useDailySuggestion(DATE, true), {
      wrapper: createHookWrapper(),
    })
    await waitFor(() => expect(result.current.data).toEqual({ kind: "none" }))
    expect(getDiscoveryRun).toHaveBeenCalledTimes(1)
  })
})

describe("run and list queries", () => {
  it("useDiscoveryRun is idle without an id", () => {
    renderHook(() => useDiscoveryRun(undefined), {
      wrapper: createHookWrapper(),
    })
    expect(getDiscoveryRun).not.toHaveBeenCalled()
  })

  it("useSuggestions passes its filters through", async () => {
    vi.mocked(listSuggestions).mockResolvedValue({ items: [suggestion] })
    const params = { status: "pending" as const, run_id: 21 }
    const { result } = renderHook(() => useSuggestions(params), {
      wrapper: createHookWrapper(),
    })
    await waitFor(() => expect(result.current.data?.items).toHaveLength(1))
    expect(listSuggestions).toHaveBeenCalledWith(params)
  })
})

describe("useDiscover", () => {
  it("starts a run and returns its id", async () => {
    vi.mocked(discover).mockResolvedValue({ run_id: 21 })
    const { result } = renderHook(() => useDiscover(), {
      wrapper: createHookWrapper(),
    })
    let out: { run_id: number } | undefined
    await act(async () => {
      out = await result.current.mutateAsync({
        mode: "explore",
        cuisine: "Thai",
      })
    })
    expect(discover).toHaveBeenCalledWith({ mode: "explore", cuisine: "Thai" })
    expect(out).toEqual({ run_id: 21 })
  })
})

describe("useAnotherSuggestion", () => {
  it("puts the answer into the daily query for that date", async () => {
    vi.mocked(anotherSuggestion).mockResolvedValue({
      kind: "running",
      runId: 22,
    })
    const qc = new QueryClient()
    const { result } = renderHook(() => useAnotherSuggestion(), {
      wrapper: createHookWrapper(qc),
    })
    await act(async () => {
      await result.current.mutateAsync(DATE)
    })
    expect(anotherSuggestion).toHaveBeenCalledWith(DATE)
    expect(qc.getQueryData(discoveryKeys.daily(DATE))).toEqual({
      kind: "running",
      runId: 22,
    })
  })
})

describe("accept and reject invalidate (F1-AC3)", () => {
  it("accept: suggestions, foods and insights", async () => {
    vi.mocked(acceptSuggestion).mockResolvedValue(
      mockSuggestion({ status: "accepted", food_id: 42 })
    )
    const keys = await invalidatedBy(useAcceptSuggestion, { id: 5, foodId: 42 })
    expect(acceptSuggestion).toHaveBeenCalledWith(5, 42)
    expect(keys).toEqual(
      expect.arrayContaining([
        discoveryKeys.all,
        foodKeys.lists(),
        insightKeys.all,
      ])
    )
  })

  it("a failed accept says the recipe is saved and Retry sends accept again", async () => {
    vi.mocked(acceptSuggestion)
      .mockRejectedValueOnce(new Error("boom"))
      .mockResolvedValueOnce(
        mockSuggestion({ status: "accepted", food_id: 42 })
      )
    const { result } = renderHook(() => useAcceptSuggestion(), {
      wrapper: createHookWrapper(),
    })
    await act(async () => {
      await result.current.mutateAsync({ id: 5, foodId: 42 }).catch(() => {})
    })
    const [message, opts] = vi.mocked(toast.error).mock.calls[0] as [
      string,
      { action: { label: string; onClick: () => void } },
    ]
    expect(message).toBe(
      "Recipe saved, but the idea couldn't be marked as added."
    )
    expect(opts.action.label).toBe("Try again")
    act(() => opts.action.onClick())
    await waitFor(() => expect(acceptSuggestion).toHaveBeenCalledTimes(2))
    expect(acceptSuggestion).toHaveBeenLastCalledWith(5, 42)
    expect(createFood).not.toHaveBeenCalled()
  })

  it("reject: suggestions, foods and insights", async () => {
    vi.mocked(rejectSuggestion).mockResolvedValue(
      mockSuggestion({ status: "rejected", reason: "taste" })
    )
    const keys = await invalidatedBy(useRejectSuggestion, {
      id: 5,
      reason: "taste" as const,
    })
    expect(rejectSuggestion).toHaveBeenCalledWith(5, "taste")
    expect(keys).toEqual(
      expect.arrayContaining([
        discoveryKeys.all,
        foodKeys.lists(),
        insightKeys.all,
      ])
    )
  })
})
