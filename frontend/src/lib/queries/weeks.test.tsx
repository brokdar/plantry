import { describe, it, expect, vi, beforeEach } from "vitest"
import { renderHook, waitFor } from "@testing-library/react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { useShoppingList, useWeekNutrition } from "@/lib/queries/weeks"
import type { ReactNode } from "react"

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
import type {
  ShoppingListResponse,
  WeekNutritionResponse,
} from "@/lib/api/weeks"

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
}

const mockShoppingList: ShoppingListResponse = {
  items: [
    { food_id: 1, name: "Chicken", total_grams: 100 },
    { food_id: 2, name: "Rice", total_grams: 200 },
  ],
}

const mockNutrition: WeekNutritionResponse = {
  days: [
    {
      day: 0,
      macros: {
        kcal: 500,
        protein: 40,
        fat: 15,
        carbs: 50,
        fiber: 5,
        sodium: 1,
      },
    },
  ],
  week: { kcal: 500, protein: 40, fat: 15, carbs: 50, fiber: 5, sodium: 1 },
}

describe("useShoppingList", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("does not fetch when weekId is 0", async () => {
    const { result } = renderHook(() => useShoppingList(0), {
      wrapper: createWrapper(),
    })
    await waitFor(() => expect(result.current.fetchStatus).toBe("idle"))
    expect(getShoppingList).not.toHaveBeenCalled()
  })

  it("fetches shopping list for a week", async () => {
    vi.mocked(getShoppingList).mockResolvedValue(mockShoppingList)

    const { result } = renderHook(() => useShoppingList(7), {
      wrapper: createWrapper(),
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
      wrapper: createWrapper(),
    })
    await waitFor(() => expect(result.current.fetchStatus).toBe("idle"))
    expect(getWeekNutrition).not.toHaveBeenCalled()
  })

  it("fetches week nutrition", async () => {
    vi.mocked(getWeekNutrition).mockResolvedValue(mockNutrition)

    const { result } = renderHook(() => useWeekNutrition(7), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(getWeekNutrition).toHaveBeenCalledWith(7)
    expect(result.current.data?.days).toHaveLength(1)
    expect(result.current.data?.days[0].day).toBe(0)
    expect(result.current.data?.week.kcal).toBe(500)
  })
})
