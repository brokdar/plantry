import { describe, it, expect, vi, beforeEach } from "vitest"
import { screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithRouter } from "@/test/render"
import { RecipeDetail } from "./RecipeDetail"
import { mockChickenCurry, mockSettingsList } from "@/test/fixtures"
import type {
  ComposedFood,
  FoodNutritionDetail,
  ScaledRecipe,
} from "@/lib/api/foods"

vi.mock("@/lib/api/foods", () => ({
  getFoodNutrition: vi.fn(),
  setFoodFavorite: vi.fn(),
  getScaled: vi.fn(),
  createVariant: vi.fn(),
  updateFood: vi.fn(),
}))

vi.mock("@/lib/api/settings", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/settings")>()),
  listSettings: vi.fn(),
}))

vi.mock("@/lib/dates", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/dates")>()),
  todayISO: () => "2026-10-31",
}))

import {
  createVariant,
  getFoodNutrition,
  getScaled,
  setFoodFavorite,
  updateFood,
} from "@/lib/api/foods"
import { listSettings } from "@/lib/api/settings"

/** Beans 200 g per reference portion and one onion per two portions. */
function scaledFor(_id: number, portions: number): Promise<ScaledRecipe> {
  const onions = portions / 2
  return Promise.resolve({
    food_id: 3,
    portions,
    reference_portions: 2,
    factor: portions / 2,
    ingredients: [
      {
        child_id: 1,
        name: "Kidney beans",
        kind: "leaf",
        amount: 200 * portions,
        unit: "g",
        grams: 200 * portions,
        display: { amount: 200 * portions, unit: "g" },
        buy: null,
      },
      {
        child_id: 2,
        name: "Onion",
        kind: "leaf",
        amount: onions,
        unit: "piece",
        grams: 120 * onions,
        display: { amount: onions, unit: "piece" },
        buy: { amount: Math.ceil(onions), unit: "piece" },
      },
    ],
  })
}

const completeNutrition: FoodNutritionDetail = {
  kcal: 412.4,
  protein: 31.6,
  fat: 12,
  carbs: 40,
  fiber: 3,
  sodium: 0.5,
  complete: true,
  missing: [],
}

const recipe: ComposedFood = {
  ...mockChickenCurry,
  notes: "Use less chili for the kids.",
  description: "A fragrant weeknight curry from northern Thailand.",
  source_url: "https://www.chefkoch.de/rezepte/123/curry.html",
}

describe("RecipeDetail", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getFoodNutrition).mockResolvedValue(completeNutrition)
    vi.mocked(getScaled).mockImplementation(scaledFor)
    vi.mocked(listSettings).mockResolvedValue(mockSettingsList())
  })

  it("renders the title, ingredients and steps", async () => {
    renderWithRouter(<RecipeDetail recipe={recipe} />)

    expect(
      await screen.findByRole("heading", { level: 1, name: "Chicken Curry" })
    ).toBeInTheDocument()
    expect(await screen.findByText("Kidney beans")).toBeInTheDocument()
    expect(screen.getByText("Cook chicken")).toBeInTheDocument()
    expect(screen.getByText("Add curry paste")).toBeInTheDocument()
  })

  it("shows kcal and protein per portion from the nutrition endpoint", async () => {
    renderWithRouter(<RecipeDetail recipe={recipe} />)

    const facts = await screen.findByTestId("recipe-facts")
    expect(await within(facts).findByText("412")).toBeInTheDocument()
    expect(within(facts).getByText("32 g")).toBeInTheDocument()
    expect(within(facts).getByText("40 min")).toBeInTheDocument()
  })

  it("keeps My notes and the imported description in separate blocks", async () => {
    renderWithRouter(<RecipeDetail recipe={recipe} />)

    const notes = await screen.findByTestId("recipe-notes")
    const description = screen.getByTestId("recipe-description")
    expect(within(notes).getByText("My notes")).toBeInTheDocument()
    expect(notes).toHaveTextContent("Use less chili for the kids.")
    expect(notes).not.toHaveTextContent("fragrant weeknight curry")
    expect(description).toHaveTextContent(
      "A fragrant weeknight curry from northern Thailand."
    )
    expect(description).not.toHaveTextContent("Use less chili")
  })

  it("omits notes and description blocks when both are empty", async () => {
    renderWithRouter(
      <RecipeDetail
        recipe={{ ...recipe, notes: null, description: null, source_url: null }}
      />
    )

    await screen.findByRole("heading", { level: 1, name: "Chicken Curry" })
    expect(screen.queryByTestId("recipe-notes")).not.toBeInTheDocument()
    expect(screen.queryByTestId("recipe-description")).not.toBeInTheDocument()
    expect(screen.queryByTestId("recipe-source")).not.toBeInTheDocument()
  })

  it("links the source in a new tab without an opener", async () => {
    renderWithRouter(<RecipeDetail recipe={recipe} />)

    const source = await screen.findByTestId("recipe-source")
    expect(source).toHaveAttribute(
      "href",
      "https://www.chefkoch.de/rezepte/123/curry.html"
    )
    expect(source).toHaveAttribute("target", "_blank")
    expect(source).toHaveAttribute("rel", "noopener noreferrer")
    expect(source).toHaveTextContent("chefkoch.de")
    expect(source).toHaveAccessibleName(/opens in a new tab/)
  })

  it("shows no completeness warning when nutrition is complete", async () => {
    renderWithRouter(<RecipeDetail recipe={recipe} />)

    await screen.findByText("412")
    expect(
      screen.queryByTestId("nutrition-completeness-warning")
    ).not.toBeInTheDocument()
  })

  it("names the incomplete ingredients and links to their editor", async () => {
    vi.mocked(getFoodNutrition).mockResolvedValue({
      ...completeNutrition,
      complete: false,
      missing: [
        { id: 7, name: "Coconut milk" },
        { id: 9, name: "Red curry paste" },
      ],
    })
    renderWithRouter(<RecipeDetail recipe={recipe} />)

    const warning = await screen.findByTestId("nutrition-completeness-warning")
    expect(
      within(warning).getByRole("link", { name: "Coconut milk" })
    ).toHaveAttribute("href", "/recipes/ingredients/7/edit")
    expect(
      within(warning).getByRole("link", { name: "Red curry paste" })
    ).toHaveAttribute("href", "/recipes/ingredients/9/edit")
    // Nothing unsaved here, so the links stay in the same tab.
    expect(
      within(warning).getByRole("link", { name: "Coconut milk" })
    ).not.toHaveAttribute("target")
  })

  it("F9-AC1: the stepper scales the ingredients without a write", async () => {
    const user = userEvent.setup()
    renderWithRouter(<RecipeDetail recipe={recipe} />)

    const list = await screen.findByTestId("recipe-ingredients-list")
    expect(await within(list).findByText("400 g")).toBeInTheDocument()
    expect(getScaled).toHaveBeenLastCalledWith(3, 2)

    const stepper = screen.getByTestId("recipe-portions")
    await user.click(
      within(stepper).getByRole("button", { name: "Increase Portions" })
    )
    expect(await within(list).findByText("600 g")).toBeInTheDocument()
    expect(getScaled).toHaveBeenLastCalledWith(3, 3)
    expect(updateFood).not.toHaveBeenCalled()
    expect(setFoodFavorite).not.toHaveBeenCalled()
  })

  it("starts the stepper at ?portions", async () => {
    renderWithRouter(<RecipeDetail recipe={recipe} portions={6} />)

    expect(await screen.findByText("1,200 g")).toBeInTheDocument()
    expect(getScaled).toHaveBeenCalledWith(3, 6)
  })

  it("shows count fractions and the buy hint", async () => {
    renderWithRouter(<RecipeDetail recipe={recipe} portions={3} />)

    const list = await screen.findByTestId("recipe-ingredients-list")
    expect(await within(list).findByText("1½ pcs")).toBeInTheDocument()
    expect(within(list).getByText("buy 2 pcs")).toBeInTheDocument()
  })

  it("F9-AC3: the history line reads B7's fields", async () => {
    renderWithRouter(
      <RecipeDetail
        recipe={{ ...recipe, times_cooked: 6, last_eaten: "2026-10-10" }}
      />
    )
    expect(await screen.findByTestId("recipe-history")).toHaveTextContent(
      "Cooked 6 times, last 3 weeks ago"
    )
  })

  it("says Not cooked yet without history", async () => {
    renderWithRouter(<RecipeDetail recipe={recipe} />)
    expect(await screen.findByTestId("recipe-history")).toHaveTextContent(
      "Not cooked yet"
    )
  })

  it("shows keep days (or the default) and freezable", async () => {
    const { unmount } = renderWithRouter(<RecipeDetail recipe={recipe} />)
    const facts = await screen.findByTestId("recipe-facts")
    expect(await within(facts).findByText("3 days")).toBeInTheDocument()
    expect(within(facts).queryByText("Freezable")).not.toBeInTheDocument()
    unmount()

    renderWithRouter(
      <RecipeDetail recipe={{ ...recipe, keep_days: 5, freezable: true }} />
    )
    const facts2 = await screen.findByTestId("recipe-facts")
    expect(within(facts2).getByText("5 days")).toBeInTheDocument()
    expect(within(facts2).getByText("Freezable")).toBeInTheDocument()
    expect(within(facts2).getByText("Yes")).toBeInTheDocument()
  })

  it("Create variant calls the variant endpoint", async () => {
    vi.mocked(createVariant).mockResolvedValue({ ...recipe, id: 99 })
    const user = userEvent.setup()
    renderWithRouter(<RecipeDetail recipe={recipe} />)

    await user.click(
      await screen.findByRole("button", { name: "Create variant" })
    )
    expect(createVariant).toHaveBeenCalledWith(3)
  })

  it("titles the steps section Steps", async () => {
    renderWithRouter(<RecipeDetail recipe={recipe} />)

    expect(
      await screen.findByRole("heading", { level: 2, name: "Steps" })
    ).toBeInTheDocument()
  })

  it("links Edit to the editor", async () => {
    renderWithRouter(<RecipeDetail recipe={recipe} />)

    expect(await screen.findByRole("link", { name: "Edit" })).toHaveAttribute(
      "href",
      "/recipes/3/edit"
    )
  })

  it("toggles favorite through the favorite mutation", async () => {
    vi.mocked(setFoodFavorite).mockResolvedValue({ ...recipe, favorite: true })
    const user = userEvent.setup()
    renderWithRouter(<RecipeDetail recipe={recipe} />)

    const favorite = await screen.findByRole("button", { name: "Favorite" })
    expect(favorite).toHaveAttribute("aria-pressed", "false")
    await user.click(favorite)
    expect(setFoodFavorite).toHaveBeenCalledWith(3, true)
  })
})
