import { describe, expect, test, vi, beforeEach } from "vitest"
import { screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { renderWithRouter } from "@/test/render"
import { mockChickenBreast, mockBrownRice } from "@/test/fixtures"

vi.mock("@/lib/api/foods", () => ({
  listFoods: vi.fn(),
  setShopSection: vi.fn(),
  setDietClass: vi.fn(),
}))

import { listFoods, setDietClass, setShopSection } from "@/lib/api/foods"
import { IngredientTable } from "./IngredientTable"

beforeEach(() => {
  vi.clearAllMocks()
})

describe("IngredientTable", () => {
  test("shows name, kcal/100 g, section, used in and source", async () => {
    vi.mocked(listFoods).mockResolvedValue({
      items: [
        {
          ...mockChickenBreast,
          used_in_count: 3,
          source: "off",
          shop_section: "frozen",
          shop_section_source: "user",
        },
        mockBrownRice,
      ],
      total: 2,
    })
    renderWithRouter(<IngredientTable />)

    const row = await screen.findByTestId(
      `ingredient-row-${mockChickenBreast.id}`
    )
    expect(
      within(row).getByRole("link", { name: "Chicken breast" })
    ).toHaveAttribute(
      "href",
      `/recipes/ingredients/${mockChickenBreast.id}/edit`
    )
    expect(row).toHaveTextContent("165")
    expect(
      within(row).getByRole("combobox", {
        name: "Store section for Chicken breast",
      })
    ).toHaveTextContent("Frozen")
    expect(row).toHaveTextContent("3 recipes")
    expect(row).toHaveTextContent("OFF")
    expect(listFoods).toHaveBeenCalledWith(
      expect.objectContaining({ kind: "leaf", sort: "name" })
    )
  })

  test("marks a classifier section as automatic", async () => {
    vi.mocked(listFoods).mockResolvedValue({
      items: [mockChickenBreast],
      total: 1,
    })
    renderWithRouter(<IngredientTable />)

    expect(
      await screen.findByRole("combobox", {
        name: "Store section for Chicken breast",
      })
    ).toHaveTextContent("Meat & fishauto")
  })

  test("changing the section calls PUT shop-section", async () => {
    const user = userEvent.setup()
    vi.mocked(listFoods).mockResolvedValue({
      items: [mockChickenBreast],
      total: 1,
    })
    vi.mocked(setShopSection).mockResolvedValue(undefined as never)
    renderWithRouter(<IngredientTable />)

    await user.click(
      await screen.findByRole("combobox", {
        name: "Store section for Chicken breast",
      })
    )
    await user.click(await screen.findByRole("option", { name: "Frozen" }))
    await vi.waitFor(() =>
      expect(setShopSection).toHaveBeenCalledWith(
        mockChickenBreast.id,
        "frozen"
      )
    )
  })

  test("Automatic reverts to the classifier", async () => {
    const user = userEvent.setup()
    vi.mocked(listFoods).mockResolvedValue({
      items: [
        {
          ...mockChickenBreast,
          shop_section: "frozen",
          shop_section_source: "user",
        },
      ],
      total: 1,
    })
    vi.mocked(setShopSection).mockResolvedValue(undefined as never)
    renderWithRouter(<IngredientTable />)

    await user.click(
      await screen.findByRole("combobox", {
        name: "Store section for Chicken breast",
      })
    )
    await user.click(await screen.findByRole("option", { name: "Automatic" }))
    await vi.waitFor(() =>
      expect(setShopSection).toHaveBeenCalledWith(mockChickenBreast.id, null)
    )
  })

  test("F6-AC1: the diet column shows the class and saves an override", async () => {
    const user = userEvent.setup()
    vi.mocked(listFoods).mockResolvedValue({
      items: [mockChickenBreast],
      total: 1,
    })
    vi.mocked(setDietClass).mockResolvedValue(undefined as never)
    renderWithRouter(<IngredientTable />)

    const select = await screen.findByRole("combobox", {
      name: "Diet for Chicken breast",
    })
    expect(select).toHaveTextContent("With meatauto")
    await user.click(select)
    await user.click(await screen.findByRole("option", { name: "Pescetarian" }))
    await vi.waitFor(() =>
      expect(setDietClass).toHaveBeenCalledWith(
        mockChickenBreast.id,
        "pescetarian"
      )
    )
  })

  test("an overridden diet class reverts with Automatic", async () => {
    const user = userEvent.setup()
    vi.mocked(listFoods).mockResolvedValue({
      items: [
        {
          ...mockChickenBreast,
          diet_class: "vegan",
          diet_class_source: "user",
        },
      ],
      total: 1,
    })
    vi.mocked(setDietClass).mockResolvedValue(undefined as never)
    renderWithRouter(<IngredientTable />)

    const select = await screen.findByRole("combobox", {
      name: "Diet for Chicken breast",
    })
    expect(select).toHaveTextContent(/^Vegan$/)
    await user.click(select)
    await user.click(await screen.findByRole("option", { name: "Automatic" }))
    await vi.waitFor(() =>
      expect(setDietClass).toHaveBeenCalledWith(mockChickenBreast.id, null)
    )
  })

  test("empty and no-result states", async () => {
    const user = userEvent.setup()
    vi.mocked(listFoods)
      .mockResolvedValueOnce({ items: [], total: 0 })
      .mockResolvedValue({ items: [], total: 0 })
    renderWithRouter(<IngredientTable />)

    expect(await screen.findByText("No ingredients yet")).toBeInTheDocument()
    await user.type(
      screen.getByRole("searchbox", { name: "Search ingredients" }),
      "xyz"
    )
    expect(await screen.findByText("No ingredients match")).toBeInTheDocument()
  })

  test("the header links to a new ingredient", async () => {
    vi.mocked(listFoods).mockResolvedValue({ items: [], total: 0 })
    renderWithRouter(<IngredientTable />)

    expect(
      await screen.findByRole("link", { name: "New ingredient" })
    ).toHaveAttribute("href", "/recipes/ingredients/new")
  })

  test("Show more loads the next page", async () => {
    const user = userEvent.setup()
    vi.mocked(listFoods).mockResolvedValue({
      items: [mockChickenBreast],
      total: 80,
    })
    renderWithRouter(<IngredientTable />)

    await user.click(await screen.findByRole("button", { name: "Show more" }))
    expect(listFoods).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 100 })
    )
  })
})
