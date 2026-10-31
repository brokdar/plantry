import { renderHook, waitFor } from "@testing-library/react"
import { QueryClient } from "@tanstack/react-query"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { mockShoppingList } from "@/test/fixtures"
import { createHookWrapper } from "@/test/render"

vi.mock("@/lib/api/shopping", () => ({ getShoppingList: vi.fn() }))

import { getShoppingList } from "@/lib/api/shopping"

import { shoppingKeys } from "./keys"
import { flattenShoppingList, useShoppingList } from "./shopping"

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
    expect(getShoppingList).toHaveBeenCalledWith(undefined)
    expect(qc.getQueryData(shoppingKeys.list())).toEqual(mockShoppingList)
  })

  it("with a range fetches it", async () => {
    vi.mocked(getShoppingList).mockResolvedValue(mockShoppingList)
    const range = { from: "2026-11-01", to: "2026-11-06" }
    const { result } = renderHook(() => useShoppingList(range), {
      wrapper: createHookWrapper(),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(getShoppingList).toHaveBeenCalledWith(range)
    expect(result.current.data?.count).toBe(2)
  })
})

describe("flattenShoppingList", () => {
  it("joins the sections into one list sorted by name", () => {
    const { items } = flattenShoppingList(mockShoppingList)
    expect(items.map((i) => i.name)).toEqual(["Kidney beans", "Onion"])
    expect(items[0].total_grams).toBe(600)
  })
})
