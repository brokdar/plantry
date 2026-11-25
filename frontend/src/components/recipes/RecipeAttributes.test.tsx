import { beforeEach, describe, expect, it, vi } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import type { ComposedFood, FoodAttributes } from "@/lib/api/foods"
import { renderWithRouter } from "@/test/render"
import { mockChickenCurry } from "@/test/fixtures"

import { RecipeAttributes } from "./RecipeAttributes"

vi.mock("@/lib/api/foods", () => ({
  getFoodAttributes: vi.fn(),
  setFoodAttributes: vi.fn(),
  revertFoodChange: vi.fn(),
}))

import { getFoodAttributes, setFoodAttributes } from "@/lib/api/foods"

const recipe: ComposedFood = {
  ...mockChickenCurry,
  children: [
    ...(mockChickenCurry.children ?? []),
    {
      ...mockChickenCurry.children![0],
      id: 9,
      child_id: 9,
      child_name: "Rice",
    },
    {
      ...mockChickenCurry.children![0],
      id: 10,
      child_id: 10,
      child_name: "Peas",
    },
    {
      ...mockChickenCurry.children![0],
      id: 11,
      child_id: 11,
      child_name: "Lime",
    },
  ],
}

const derived: FoodAttributes = {
  food_id: recipe.id,
  version: 0,
  attributes: {
    cuisine: "indian",
    heroes: ["Chicken breast"],
    saucy: true,
    richness: 2,
    colour: "orange",
  },
  sources: {
    cuisine: "rule",
    heroes: "rule",
    saucy: "rule",
    richness: "rule",
    colour: "rule",
  },
}

describe("RecipeAttributes", () => {
  beforeEach(() => {
    vi.mocked(getFoodAttributes).mockReset().mockResolvedValue(derived)
    vi.mocked(setFoodAttributes)
      .mockReset()
      .mockResolvedValue({
        ...derived,
        attributes: { ...derived.attributes, cuisine: "thai" },
        sources: { ...derived.sources, cuisine: "user" },
        undo: { first_event_id: 1, last_event_id: 1 },
      })
  })

  it("shows every attribute, unknown ones as a dash", async () => {
    renderWithRouter(<RecipeAttributes recipe={recipe} />)
    expect(await screen.findByTestId("attr-chip-cuisine")).toHaveTextContent(
      "CuisineIndian"
    )
    expect(screen.getByTestId("attr-chip-saucy")).toHaveTextContent("Saucy")
    expect(screen.getByTestId("attr-chip-richness")).toHaveTextContent("Rich")
    expect(screen.getByTestId("attr-chip-heroes")).toHaveTextContent(
      "Chicken breast"
    )
    expect(screen.getByTestId("attr-chip-kind")).toHaveTextContent("Dish–")
    expect(screen.getByTestId("attr-chip-kind")).toHaveAttribute(
      "data-source",
      "none"
    )
  })

  it("picking a value saves it as the user's; automatic resets it", async () => {
    const user = userEvent.setup()
    renderWithRouter(<RecipeAttributes recipe={recipe} />)
    await user.click(await screen.findByTestId("attr-chip-cuisine"))
    expect(screen.getByTestId("attr-reset")).toBeDisabled()
    await user.click(screen.getByTestId("attr-option-thai"))
    expect(setFoodAttributes).toHaveBeenCalledWith(recipe.id, {
      cuisine: "thai",
    })
    const chip = screen.getByTestId("attr-chip-cuisine")
    await waitFor(() => expect(chip).toHaveTextContent("Thai"))
    expect(chip).toHaveAttribute("data-source", "user")
    expect(within(chip).getByText("set by you")).toBeInTheDocument()

    await user.click(chip)
    await user.click(screen.getByTestId("attr-reset"))
    expect(setFoodAttributes).toHaveBeenLastCalledWith(recipe.id, {
      cuisine: null,
    })
  })

  it("saucy and richness send their typed values", async () => {
    const user = userEvent.setup()
    renderWithRouter(<RecipeAttributes recipe={recipe} />)
    await user.click(await screen.findByTestId("attr-chip-saucy"))
    await user.click(screen.getByTestId("attr-option-false"))
    expect(setFoodAttributes).toHaveBeenCalledWith(recipe.id, { saucy: false })
    await user.click(await screen.findByTestId("attr-chip-richness"))
    await user.click(screen.getByTestId("attr-option-0"))
    expect(setFoodAttributes).toHaveBeenLastCalledWith(recipe.id, {
      richness: 0,
    })
  })

  it("heroes pick at most three of the ingredients", async () => {
    const user = userEvent.setup()
    renderWithRouter(<RecipeAttributes recipe={recipe} />)
    await user.click(await screen.findByTestId("attr-chip-heroes"))
    await user.click(screen.getByRole("checkbox", { name: "Rice" }))
    await user.click(screen.getByRole("checkbox", { name: "Peas" }))
    expect(screen.getByRole("checkbox", { name: "Lime" })).toBeDisabled()
    await user.click(screen.getByTestId("attr-save"))
    expect(setFoodAttributes).toHaveBeenCalledWith(recipe.id, {
      heroes: ["Chicken breast", "Rice", "Peas"],
    })
  })

  it("taste saves the whole profile", async () => {
    const user = userEvent.setup()
    renderWithRouter(<RecipeAttributes recipe={recipe} />)
    await user.click(await screen.findByTestId("attr-chip-taste"))
    const spicy = screen.getByRole("group", { name: "Spicy" })
    await user.click(within(spicy).getByRole("button", { name: "3" }))
    await user.click(screen.getByTestId("attr-save"))
    expect(setFoodAttributes).toHaveBeenCalledWith(recipe.id, {
      taste: { salt: 0, acid: 0, sweet: 0, bitter: 0, umami: 0, heat: 3 },
    })
  })
})
