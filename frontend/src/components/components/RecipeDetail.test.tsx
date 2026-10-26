import { describe, it, expect, vi, beforeEach } from "vitest"
import { act, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithRouter } from "@/test/render"
import i18n from "@/lib/i18n"
import { RecipeDetail } from "./RecipeDetail"
import { mockChickenCurry } from "@/test/fixtures"
import type { ComposedFood, FoodNutritionDetail } from "@/lib/api/foods"

vi.mock("@/lib/api/foods", () => ({
  getFoodNutrition: vi.fn(),
  setFoodFavorite: vi.fn(),
}))

import { getFoodNutrition, setFoodFavorite } from "@/lib/api/foods"

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
    vi.mocked(getFoodNutrition).mockResolvedValue(completeNutrition)
  })

  it("renders the title, ingredients and steps", async () => {
    renderWithRouter(<RecipeDetail recipe={recipe} />)

    expect(
      await screen.findByRole("heading", { level: 1, name: "Chicken Curry" })
    ).toBeInTheDocument()
    expect(screen.getByText("Chicken breast")).toBeInTheDocument()
    expect(screen.getByText("Cook chicken")).toBeInTheDocument()
    expect(screen.getByText("Add curry paste")).toBeInTheDocument()
  })

  it("shows kcal and protein per portion from the nutrition endpoint", async () => {
    renderWithRouter(<RecipeDetail recipe={recipe} />)

    const facts = await screen.findByTestId("recipe-facts")
    expect(await within(facts).findByText("412")).toBeInTheDocument()
    expect(within(facts).getByText("32 g")).toBeInTheDocument()
    expect(within(facts).getByText("2")).toBeInTheDocument()
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
    ).toHaveAttribute("href", "/ingredients/7/edit")
    expect(
      within(warning).getByRole("link", { name: "Red curry paste" })
    ).toHaveAttribute("href", "/ingredients/9/edit")
    // Nothing unsaved here, so the links stay in the same tab.
    expect(
      within(warning).getByRole("link", { name: "Coconut milk" })
    ).not.toHaveAttribute("target")
  })

  it("shows translated kitchen units and keeps metric symbols", async () => {
    const withUnits: ComposedFood = {
      ...recipe,
      children: [
        ...(recipe.children ?? []),
        {
          ...recipe.children![0]!,
          id: 2,
          child_id: 2,
          child_name: "Coconut milk",
          amount: 1,
          unit: "can",
        },
        {
          ...recipe.children![0]!,
          id: 3,
          child_id: 3,
          child_name: "Fish sauce",
          amount: 2,
          unit: "tbsp",
        },
        {
          ...recipe.children![0]!,
          id: 4,
          child_id: 4,
          child_name: "Lime juice",
          amount: 1,
          unit: "tbsp",
        },
      ],
    }
    renderWithRouter(<RecipeDetail recipe={withUnits} />)

    expect(await screen.findByText("300 g")).toBeInTheDocument()
    expect(screen.getByText("1 can")).toBeInTheDocument()
    expect(screen.getByText("2 tablespoons")).toBeInTheDocument()
    expect(screen.getByText("1 tablespoon")).toBeInTheDocument()

    await act(() => i18n.changeLanguage("de"))
    try {
      expect(await screen.findByText("2 Esslöffel")).toBeInTheDocument()
      expect(screen.getByText("1 Dose")).toBeInTheDocument()
      expect(screen.getByText("1 Esslöffel")).toBeInTheDocument()
      expect(screen.getByText("300 g")).toBeInTheDocument()
    } finally {
      await act(() => i18n.changeLanguage("en"))
    }
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
      "/components/3/edit"
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
