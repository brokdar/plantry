import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { act, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { renderWithRouter } from "@/test/render"
import { mockChickenCurry } from "@/test/fixtures"
import { ApiError } from "@/lib/api/client"

vi.mock("@/lib/api/foods", () => ({
  getFood: vi.fn(),
  getFoodNutrition: vi.fn(),
  setFoodFavorite: vi.fn(),
}))

import { getFood, getFoodNutrition } from "@/lib/api/foods"

import { RecipeDetailPage } from "./RecipeDetailPage"

describe("RecipeDetailPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getFoodNutrition).mockResolvedValue({
      kcal: 400,
      protein: 30,
      fat: 10,
      carbs: 40,
      fiber: 3,
      sodium: 0,
      complete: true,
      missing: [],
    })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it("shows a not-found state for an unknown id without retrying", async () => {
    vi.mocked(getFood).mockRejectedValue(
      new ApiError(404, "error.food.not_found")
    )
    renderWithRouter(<RecipeDetailPage id="999" />)

    expect(
      await screen.findByRole("heading", { level: 1, name: "Recipe not found" })
    ).toBeInTheDocument()
    expect(getFood).toHaveBeenCalledTimes(1)
    expect(
      screen.getByRole("link", { name: "Back to recipes" })
    ).toHaveAttribute("href", "/recipes")
    expect(
      screen.queryByRole("button", { name: "Try again" })
    ).not.toBeInTheDocument()
  })

  it("shows a load error with a retry for other failures", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    vi.mocked(getFood).mockRejectedValue(new ApiError(500, "error.server"))
    const user = userEvent.setup({
      advanceTimers: vi.advanceTimersByTime.bind(vi),
    })
    renderWithRouter(<RecipeDetailPage id="3" />)

    // Transient errors keep the default retries before giving up.
    await act(() => vi.advanceTimersByTimeAsync(10_000))
    const alert = await screen.findByRole("alert")
    expect(alert).toHaveTextContent("Couldn't load this recipe")
    expect(getFood).toHaveBeenCalledTimes(4)
    expect(screen.getByRole("link", { name: "Back to recipes" })).toBeVisible()

    vi.mocked(getFood).mockResolvedValue(mockChickenCurry)
    await user.click(screen.getByRole("button", { name: "Try again" }))
    expect(
      await screen.findByRole("heading", { level: 1, name: "Chicken Curry" })
    ).toBeInTheDocument()
  })
})
