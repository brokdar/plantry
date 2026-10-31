import { renderHook, waitFor } from "@testing-library/react"
import { QueryClient } from "@tanstack/react-query"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { mockShoppingList } from "@/test/fixtures"
import { createHookWrapper } from "@/test/render"

vi.mock("@/lib/api/shopping", () => ({ getShoppingList: vi.fn() }))

import { getShoppingList } from "@/lib/api/shopping"

import { shoppingKeys } from "./keys"
import { useShoppingList } from "./shopping"

beforeEach(() => vi.clearAllMocks())

describe("useShoppingList", () => {
  it("without a range asks for the server default", async () => {
    vi.mocked(getShoppingList).mockResolvedValue(mockShoppingList)
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const { result } = renderHook(() => useShoppingList(), {
      wrapper: createHookWrapper(qc),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(getShoppingList).toHaveBeenCalledWith(undefined, false)
    expect(qc.getQueryData(shoppingKeys.list())).toEqual(mockShoppingList)
  })

  it("with a range fetches it", async () => {
    vi.mocked(getShoppingList).mockResolvedValue(mockShoppingList)
    const range = { from: "2026-11-01", to: "2026-11-06" }
    const { result } = renderHook(() => useShoppingList(range), {
      wrapper: createHookWrapper(),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(getShoppingList).toHaveBeenCalledWith(range, false)
    expect(result.current.data?.count).toBe(2)
  })

  // Phase 3 F5: drafts included is its own list under its own key.
  it("with includeDrafts asks for drafts under its own key", async () => {
    vi.mocked(getShoppingList).mockResolvedValue(mockShoppingList)
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const range = { from: "2026-11-01", to: "2026-11-06" }
    const { result } = renderHook(() => useShoppingList(range, true), {
      wrapper: createHookWrapper(qc),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(getShoppingList).toHaveBeenCalledWith(range, true)
    expect(qc.getQueryData(shoppingKeys.list(range, true))).toEqual(
      mockShoppingList
    )
    expect(qc.getQueryData(shoppingKeys.list(range))).toBeUndefined()
  })
})
