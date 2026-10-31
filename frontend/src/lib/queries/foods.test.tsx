import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { renderHook, waitFor } from "@testing-library/react"

import { queryClient } from "@/lib/query-client"
import {
  mockChickenBreast,
  mockChickenCurry,
  mockScaled,
} from "@/test/fixtures"
import { createHookWrapper } from "@/test/render"

import {
  useCreateFood,
  useCreateVariant,
  useDeleteFood,
  useScaled,
  useSetDietClass,
  useSetShopSection,
} from "./foods"
import { foodKeys, planKeys, setupKeys, shoppingKeys } from "./keys"

vi.mock("@/lib/api/foods", () => ({
  createFood: vi.fn(),
  deleteFood: vi.fn(),
  createVariant: vi.fn(),
  getScaled: vi.fn(),
  setDietClass: vi.fn(),
  setShopSection: vi.fn(),
}))

import {
  createFood,
  createVariant,
  deleteFood,
  getScaled,
  setDietClass,
  setShopSection,
} from "@/lib/api/foods"

// The Generate gate and the guided Recipes state read the recipe count from
// GET /api/setup, so every mutation that changes it must refresh that query.
describe("recipe-count mutations invalidate the setup status", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue()
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("useCreateFood", async () => {
    vi.mocked(createFood).mockResolvedValue(mockChickenCurry)
    const { result } = renderHook(() => useCreateFood(), {
      wrapper: createHookWrapper(),
    })
    result.current.mutate({ kind: "composed", name: "Curry" } as never)
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({
      queryKey: setupKeys.status(),
    })
  })

  it("useDeleteFood", async () => {
    vi.mocked(deleteFood).mockResolvedValue(undefined)
    const { result } = renderHook(() => useDeleteFood(), {
      wrapper: createHookWrapper(),
    })
    result.current.mutate(mockChickenCurry.id)
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({
      queryKey: setupKeys.status(),
    })
  })

  it("useCreateVariant", async () => {
    vi.mocked(createVariant).mockResolvedValue(mockChickenCurry)
    const { result } = renderHook(() => useCreateVariant(), {
      wrapper: createHookWrapper(),
    })
    result.current.mutate(mockChickenCurry.id)
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({
      queryKey: setupKeys.status(),
    })
  })
})

describe("useScaled", () => {
  beforeEach(() => vi.clearAllMocks())

  it("keeps the previous list while new portions load", async () => {
    let resolveSecond: (v: typeof mockScaled) => void = () => {}
    vi.mocked(getScaled)
      .mockResolvedValueOnce(mockScaled)
      .mockImplementationOnce(
        () => new Promise((resolve) => (resolveSecond = resolve))
      )
    const { result, rerender } = renderHook(
      ({ portions }) => useScaled(42, portions),
      { wrapper: createHookWrapper(), initialProps: { portions: 6 } }
    )
    await waitFor(() => expect(result.current.data).toEqual(mockScaled))

    rerender({ portions: 8 })
    expect(getScaled).toHaveBeenLastCalledWith(42, 8)
    expect(result.current.data).toEqual(mockScaled)
    expect(result.current.isPlaceholderData).toBe(true)

    const eight = { ...mockScaled, portions: 8, factor: 2 }
    resolveSecond(eight)
    await waitFor(() => expect(result.current.data).toEqual(eight))
  })

  it("waits for positive portions", () => {
    renderHook(() => useScaled(42, 0), { wrapper: createHookWrapper() })
    expect(getScaled).not.toHaveBeenCalled()
  })
})

// F1-AC2: the shop section moves an item on the list and changes the food.
describe("useSetShopSection", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue()
  })
  afterEach(() => vi.restoreAllMocks())

  it("invalidates the shopping list and the food", async () => {
    vi.mocked(setShopSection).mockResolvedValue(mockChickenBreast)
    const { result } = renderHook(() => useSetShopSection(), {
      wrapper: createHookWrapper(),
    })
    result.current.mutate({ id: 1, section: "frozen" })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(setShopSection).toHaveBeenCalledWith(1, "frozen")
    expect(
      vi
        .mocked(queryClient.invalidateQueries)
        .mock.calls.map((c) => c[0]?.queryKey)
    ).toEqual([shoppingKeys.all, foodKeys.lists(), foodKeys.detail(1)])
  })
})

describe("useSetDietClass", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue()
  })
  afterEach(() => vi.restoreAllMocks())

  it.each(["vegan", null] as const)(
    "sets %s and refreshes every food and the plan",
    async (dietClass) => {
      vi.mocked(setDietClass).mockResolvedValue(mockChickenBreast)
      const { result } = renderHook(() => useSetDietClass(), {
        wrapper: createHookWrapper(),
      })
      result.current.mutate({ id: 1, dietClass })
      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(setDietClass).toHaveBeenCalledWith(1, dietClass)
      expect(
        vi
          .mocked(queryClient.invalidateQueries)
          .mock.calls.map((c) => c[0]?.queryKey)
      ).toEqual([foodKeys.all, planKeys.all])
    }
  )
})
