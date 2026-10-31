import { renderHook, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { mockInsights, mockRunsPage } from "@/test/fixtures"
import { createHookWrapper } from "@/test/render"

vi.mock("@/lib/api/runs", () => ({ listRuns: vi.fn(), getRun: vi.fn() }))
vi.mock("@/lib/api/insights", () => ({ getInsights: vi.fn() }))
vi.mock("@/lib/api/prices", () => ({ getPrice: vi.fn() }))

import { getInsights } from "@/lib/api/insights"
import { getPrice } from "@/lib/api/prices"
import { getRun, listRuns, type RunDetail } from "@/lib/api/runs"

import { useModelPrice, useRunDetail, useRunInsights, useRuns } from "./runs"

beforeEach(() => vi.clearAllMocks())

const detail: RunDetail = {
  ...mockRunsPage.items[0],
  scope: { kind: "window" },
  instruction: null,
  notes: null,
  violations: null,
  error_key: null,
  steps: [
    {
      seq: 1,
      type: "llm_call",
      name: "fake",
      args: '{"messages":',
      args_truncated: true,
      result: null,
      result_truncated: false,
      is_error: false,
      duration_ms: 120,
      tokens: { input: 1200, output: 300 },
    },
  ],
  prompt_text: null,
}

describe("run log and insights queries", () => {
  it("useRuns fetches one page with the given filters", async () => {
    vi.mocked(listRuns).mockResolvedValue(mockRunsPage)
    const params = { kind: "generate" as const, cursor: 11 }
    const { result } = renderHook(() => useRuns(params), {
      wrapper: createHookWrapper(),
    })
    await waitFor(() => expect(result.current.data).toEqual(mockRunsPage))
    expect(listRuns).toHaveBeenCalledWith(params)
  })

  it("useRunDetail fetches the open run only", async () => {
    vi.mocked(getRun).mockResolvedValue(detail)
    const { result, rerender } = renderHook(
      ({ id }: { id: number | undefined }) => useRunDetail(id),
      {
        wrapper: createHookWrapper(),
        initialProps: { id: undefined as number | undefined },
      }
    )
    expect(getRun).not.toHaveBeenCalled()
    rerender({ id: 12 })
    await waitFor(() => expect(result.current.data).toEqual(detail))
    expect(getRun).toHaveBeenCalledWith(12)
  })

  it("useRunInsights fetches the weekly summary", async () => {
    vi.mocked(getInsights).mockResolvedValue(mockInsights)
    const { result } = renderHook(
      () => useRunInsights({ from: "2026-10-01" }),
      { wrapper: createHookWrapper() }
    )
    await waitFor(() => expect(result.current.data).toEqual(mockInsights))
    expect(getInsights).toHaveBeenCalledWith({ from: "2026-10-01" })
  })

  it("useModelPrice asks only with a model", async () => {
    const price = {
      model: "m",
      price: null,
      source: "none" as const,
      as_of: "2026-10-01",
    }
    vi.mocked(getPrice).mockResolvedValue(price)
    const { result, rerender } = renderHook(
      ({ model }: { model: string }) => useModelPrice(model),
      { wrapper: createHookWrapper(), initialProps: { model: "" } }
    )
    expect(getPrice).not.toHaveBeenCalled()
    rerender({ model: "m" })
    await waitFor(() => expect(result.current.data).toEqual(price))
    expect(getPrice).toHaveBeenCalledWith("m")
  })
})
