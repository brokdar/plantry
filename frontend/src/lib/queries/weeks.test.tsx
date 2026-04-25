import { describe, it, expect, vi, beforeEach } from "vitest"
import { renderHook, waitFor } from "@testing-library/react"
import { useShoppingList, useWeekNutrition } from "@/lib/queries/weeks"
import { createHookWrapper } from "@/test/render"
import { mockShoppingList, mockWeekNutrition } from "@/test/fixtures"

vi.mock("@/lib/api/weeks", () => ({
  getCurrentWeek: vi.fn(),
  getWeek: vi.fn(),
  getWeekByDate: vi.fn(),
  getShoppingList: vi.fn(),
  getWeekNutrition: vi.fn(),
  copyWeek: vi.fn(),
  createPlate: vi.fn(),
  listWeeks: vi.fn(),
}))

import { getShoppingList, getWeekNutrition } from "@/lib/api/weeks"

describe("useShoppingList", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("does not fetch when weekId is 0", async () => {
    const { result } = renderHook(() => useShoppingList(0), {
      wrapper: createHookWrapper(),
    })
    await waitFor(() => expect(result.current.fetchStatus).toBe("idle"))
    expect(getShoppingList).not.toHaveBeenCalled()
  })

  it("fetches shopping list for a week", async () => {
    vi.mocked(getShoppingList).mockResolvedValue(mockShoppingList)

    const { result } = renderHook(() => useShoppingList(7), {
      wrapper: createHookWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(getShoppingList).toHaveBeenCalledWith(7)
    expect(result.current.data?.items).toHaveLength(2)
    expect(result.current.data?.items[0].name).toBe("Chicken")
  })
})

describe("useWeekNutrition", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("does not fetch when weekId is 0", async () => {
    const { result } = renderHook(() => useWeekNutrition(0), {
      wrapper: createHookWrapper(),
    })
    await waitFor(() => expect(result.current.fetchStatus).toBe("idle"))
    expect(getWeekNutrition).not.toHaveBeenCalled()
  })

  it("fetches week nutrition", async () => {
    vi.mocked(getWeekNutrition).mockResolvedValue(mockWeekNutrition)

    const { result } = renderHook(() => useWeekNutrition(7), {
      wrapper: createHookWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(getWeekNutrition).toHaveBeenCalledWith(7)
    expect(result.current.data?.days).toHaveLength(1)
    expect(result.current.data?.days[0].day).toBe(0)
    expect(result.current.data?.week.kcal).toBe(500)
  })
})
