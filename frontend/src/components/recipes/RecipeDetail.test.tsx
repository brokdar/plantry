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
  setDietClass: vi.fn(),
}))

vi.mock("@/lib/api/plan", () => ({ getPlan: vi.fn() }))

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
  setDietClass,
  setFoodFavorite,
  updateFood,
} from "@/lib/api/foods"
import { getPlan } from "@/lib/api/plan"
import { mockPlan } from "@/test/fixtures"
import { listSettings } from "@/lib/api/settings"
import { rememberLibrarySearch } from "@/lib/recipes/library"

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
        diet_class: "vegan",
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
        diet_class: "vegan",
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

  it("writes the stepper's portions back to the URL", async () => {
    const user = userEvent.setup()
    const onPortionsChange = vi.fn()
    renderWithRouter(
      <RecipeDetail recipe={recipe} onPortionsChange={onPortionsChange} />
    )
    await user.click(
      within(await screen.findByTestId("recipe-portions")).getByRole("button", {
        name: "Increase Portions",
      })
    )
    expect(onPortionsChange).toHaveBeenCalledWith(3)
  })

  it("goes back to the library with its last search", async () => {
    rememberLibrarySearch({ q: "curry", fav: true, add: "url" })
    renderWithRouter(<RecipeDetail recipe={recipe} />)
    expect(
      await screen.findByRole("link", { name: "Back to recipes" })
    ).toHaveAttribute("href", "/recipes?q=curry&fav=true")
  })

  it("F9-AC4: Cook now opens cook mode with the stepper's portions", async () => {
    const user = userEvent.setup()
    renderWithRouter(<RecipeDetail recipe={recipe} />)

    const cook = await screen.findByRole("link", { name: "Cook now" })
    expect(cook).toHaveAttribute("href", "/recipes/3/cook?portions=2")
    await user.click(
      within(screen.getByTestId("recipe-portions")).getByRole("button", {
        name: "Increase Portions",
      })
    )
    expect(cook).toHaveAttribute("href", "/recipes/3/cook?portions=3")
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

  it("says Cooked N times without a last-eaten date", async () => {
    renderWithRouter(
      <RecipeDetail recipe={{ ...recipe, times_cooked: 2, last_eaten: null }} />
    )
    expect(await screen.findByTestId("recipe-history")).toHaveTextContent(
      /^Cooked 2 times$/
    )
  })

  it("shows an error with Retry when scaling fails, and Retry recovers", async () => {
    vi.mocked(getScaled).mockRejectedValueOnce(new Error("offline"))
    const user = userEvent.setup()
    renderWithRouter(<RecipeDetail recipe={recipe} />)

    const error = await screen.findByTestId("recipe-ingredients-error")
    expect(error).toHaveTextContent("The ingredients didn't load.")
    await user.click(within(error).getByRole("button", { name: "Try again" }))
    expect(await screen.findByText("400 g")).toBeInTheDocument()
    expect(screen.queryByTestId("recipe-ingredients-error")).toBeNull()
  })

  it("shows a skeleton while the scaled list loads", async () => {
    vi.mocked(getScaled).mockReturnValueOnce(new Promise(() => {}))
    renderWithRouter(<RecipeDetail recipe={recipe} />)
    expect(
      await screen.findByTestId("recipe-ingredients-loading")
    ).toBeInTheDocument()
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

  it("F6-AC1: shows the diet class in the facts and saves an override", async () => {
    vi.mocked(setDietClass).mockResolvedValue(recipe)
    const user = userEvent.setup()
    renderWithRouter(<RecipeDetail recipe={recipe} />)

    const diet = await screen.findByRole("combobox", {
      name: "Diet for Chicken Curry",
    })
    expect(diet).toHaveTextContent("With meatauto")
    expect(screen.queryByTestId("diet-unknown-warning")).toBeNull()
    await user.click(diet)
    await user.click(await screen.findByRole("option", { name: "Vegetarian" }))
    await vi.waitFor(() =>
      expect(setDietClass).toHaveBeenCalledWith(3, "vegetarian")
    )
  })

  it("warns about an unknown diet and links the unknown ingredients", async () => {
    vi.mocked(getScaled).mockImplementation(async (id, portions) => {
      const scaled = await scaledFor(id, portions)
      scaled.ingredients[1] = {
        ...scaled.ingredients[1],
        diet_class: "unknown",
      }
      return scaled
    })
    renderWithRouter(
      <RecipeDetail
        recipe={{
          ...recipe,
          diet_class: "unknown",
          diet_class_source: "derived",
        }}
      />
    )

    const warning = await screen.findByTestId("diet-unknown-warning")
    expect(warning).toHaveTextContent("Diet unknown")
    expect(
      within(warning).getByRole("link", { name: "Onion" })
    ).toHaveAttribute("href", "/recipes/ingredients/2/edit")
    expect(
      within(warning).queryByRole("link", { name: "Kidney beans" })
    ).toBeNull()
  })

  it("Add to plan opens the cell picker sheet", async () => {
    vi.mocked(getPlan).mockResolvedValue(mockPlan)
    const user = userEvent.setup()
    renderWithRouter(<RecipeDetail recipe={recipe} />)

    await user.click(await screen.findByRole("button", { name: "Add to plan" }))
    expect(await screen.findByTestId("add-to-plan-sheet")).toBeInTheDocument()
    expect(await screen.findAllByRole("radio")).not.toHaveLength(0)
  })
})
