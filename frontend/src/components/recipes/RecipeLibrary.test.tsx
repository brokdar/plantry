import { beforeEach, describe, it, expect, vi } from "vitest"
import { screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { renderWithRouter } from "@/test/render"
import { mockChickenCurry, mockTofuBowl } from "@/test/fixtures"
import type { LibrarySearch } from "@/lib/recipes/library"

import { RecipeLibrary } from "./RecipeLibrary"

vi.mock("@/lib/api/foods", () => ({
  listFoods: vi.fn(),
}))

vi.mock("@/lib/api/setup", () => ({
  getSetupStatus: vi.fn(),
  completeSetup: vi.fn(),
}))

vi.mock("@/lib/dates", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/dates")>()),
  todayISO: () => "2026-10-31",
}))

import { listFoods } from "@/lib/api/foods"
import { getSetupStatus } from "@/lib/api/setup"

function setupWith(recipe_count: number) {
  vi.mocked(getSetupStatus).mockResolvedValue({
    completed: true,
    recipe_count,
    recipe_target: 10,
  })
}

function renderLibrary(search: LibrarySearch = {}) {
  const onSearchChange = vi.fn()
  renderWithRouter(
    <RecipeLibrary search={search} onSearchChange={onSearchChange} />
  )
  return onSearchChange
}

describe("RecipeLibrary", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    setupWith(10)
  })

  it("shows time, kcal per portion and last eaten on each card", async () => {
    vi.mocked(listFoods).mockResolvedValue({
      items: [
        {
          ...mockChickenCurry,
          last_eaten: "2026-10-10",
          per_portion: { kcal: 520.4, protein: 30, complete: true },
        },
        mockTofuBowl,
      ],
      total: 2,
    })

    renderLibrary()

    const curry = await screen.findByTestId(
      `recipe-card-${mockChickenCurry.id}-meta`
    )
    expect(curry).toHaveTextContent("40 min · 520 kcal · eaten 3 weeks ago")
    expect(
      screen.getByTestId(`recipe-card-${mockTofuBowl.id}-meta`)
    ).toHaveTextContent("20 min · never eaten")
    // The diet icon leads the meta line, named for screen readers.
    expect(
      within(curry).getByRole("img", { name: "Diet: With meat" })
    ).toBeInTheDocument()
    expect(
      within(
        screen.getByTestId(`recipe-card-${mockTofuBowl.id}-meta`)
      ).getByRole("img", { name: "Diet: Vegan" })
    ).toBeInTheDocument()
    expect(screen.getByRole("link", { name: /Chicken Curry/ })).toHaveAttribute(
      "href",
      `/recipes/${mockChickenCurry.id}`
    )
  })

  it("draws the initial as the placeholder when there is no photo", async () => {
    vi.mocked(listFoods).mockResolvedValue({
      items: [mockTofuBowl],
      total: 1,
    })

    renderLibrary()

    const card = await screen.findByTestId(`recipe-card-${mockTofuBowl.id}`)
    expect(card.querySelector("img")).toBeNull()
    expect(card).toHaveTextContent(/^T/)
  })

  it("maps the URL filters onto the list request", async () => {
    vi.mocked(listFoods).mockResolvedValue({ items: [], total: 0 })

    renderLibrary({
      role: "main",
      diet: "vegetarian",
      quick: true,
      stale: true,
      fav: true,
      new: true,
      sort: "last_eaten",
    })

    await screen.findByTestId("recipes-empty")
    expect(listFoods).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: "composed",
        role: "main",
        diet: "vegetarian",
        max_minutes: 30,
        not_eaten_weeks: 4,
        favorite: 1,
        created_within_days: 30,
        sort: "last_eaten",
      })
    )
    expect(
      screen.getByRole("button", { name: "Not eaten lately" })
    ).toHaveAttribute("aria-pressed", "true")
    expect(screen.getByText("No recipes match")).toBeInTheDocument()
    expect(screen.getByRole("combobox", { name: "Diet" })).toHaveTextContent(
      "Vegetarian"
    )
  })

  it("F6-AC1: the Diet chip writes ?diet= without a With meat choice", async () => {
    vi.mocked(listFoods).mockResolvedValue({ items: [], total: 0 })
    const user = userEvent.setup()

    const onSearchChange = renderLibrary({ sort: "created" })
    await user.click(await screen.findByRole("combobox", { name: "Diet" }))
    expect(
      screen.queryByRole("option", { name: "With meat" })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole("option", { name: "Pescetarian" }))
    expect(onSearchChange).toHaveBeenLastCalledWith(
      { sort: "created", diet: "pescetarian" },
      undefined
    )
  })

  it("a chip toggles its filter in the URL", async () => {
    vi.mocked(listFoods).mockResolvedValue({ items: [], total: 0 })
    const user = userEvent.setup()

    const onSearchChange = renderLibrary({ sort: "created" })
    await user.click(
      await screen.findByRole("button", { name: "Not eaten lately" })
    )
    expect(onSearchChange).toHaveBeenLastCalledWith(
      { sort: "created", stale: true },
      undefined
    )
  })

  it("clearing the filters keeps the sort", async () => {
    vi.mocked(listFoods).mockResolvedValue({ items: [], total: 0 })
    const user = userEvent.setup()

    const onSearchChange = renderLibrary({ fav: true, sort: "created" })
    await user.click(
      await screen.findByRole("button", { name: "Clear filters" })
    )
    expect(onSearchChange).toHaveBeenLastCalledWith({ sort: "created" })
  })

  it("typing in the search replaces the URL entry", async () => {
    vi.mocked(listFoods).mockResolvedValue({ items: [], total: 0 })
    const user = userEvent.setup()

    const onSearchChange = renderLibrary()
    await user.type(
      await screen.findByRole("searchbox", { name: "Search recipes" }),
      "c"
    )
    expect(onSearchChange).toHaveBeenLastCalledWith({ q: "c" }, true)
  })

  it("editing in the middle of the search keeps the caret", async () => {
    vi.mocked(listFoods).mockResolvedValue({ items: [], total: 0 })
    const user = userEvent.setup()

    const onSearchChange = renderLibrary({ q: "chicken soup" })
    const box = await screen.findByRole("searchbox", { name: "Search recipes" })
    await user.click(box)
    await user.keyboard("{Home}{ArrowRight}{ArrowRight}XY")
    expect(box).toHaveValue("chXYicken soup")
    expect(onSearchChange).toHaveBeenLastCalledWith(
      { q: "chXYicken soup" },
      true
    )
  })

  it("the Add menu offers import from a link and create manually", async () => {
    vi.mocked(listFoods).mockResolvedValue({ items: [], total: 0 })
    const user = userEvent.setup()

    renderLibrary()
    await user.click(await screen.findByRole("button", { name: "Add recipe" }))
    const menu = await screen.findByRole("menu")
    expect(
      within(menu).getByRole("menuitem", { name: "Import from a link" })
    ).toHaveAttribute("href", "/recipes?add=url")
    expect(
      within(menu).getByRole("menuitem", { name: "Create manually" })
    ).toHaveAttribute("href", "/recipes/new")
  })

  it("has Recipes and Ingredients tabs", async () => {
    vi.mocked(listFoods).mockResolvedValue({ items: [], total: 0 })

    renderLibrary()

    const tabs = await screen.findByRole("navigation", {
      name: "Recipe sections",
    })
    expect(
      within(tabs).getByRole("link", { name: "Ingredients" })
    ).toHaveAttribute("href", "/recipes/ingredients")
    expect(within(tabs).getByRole("link", { name: "Recipes" })).toHaveAttribute(
      "href",
      "/recipes"
    )
  })

  it("opens the import sheet for add=url and closing drops it", async () => {
    vi.mocked(listFoods).mockResolvedValue({ items: [], total: 0 })
    const user = userEvent.setup()

    const onSearchChange = renderLibrary({ add: "url", fav: true })
    const sheet = await screen.findByRole("dialog", { name: "Import recipe" })
    expect(within(sheet).getByLabelText("Recipe URL")).toBeInTheDocument()

    await user.click(within(sheet).getByRole("button", { name: "Close" }))
    expect(onSearchChange).toHaveBeenLastCalledWith(
      { add: undefined, fav: true },
      undefined
    )
  })

  it("an empty library shows the onboarding cards in place of the empty grid", async () => {
    setupWith(0)
    vi.mocked(listFoods).mockResolvedValue({ items: [], total: 0 })

    renderLibrary()

    expect(
      await screen.findByText(
        "0 of 10 recipes. Plantry needs 10 to plan a varied week."
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole("heading", { name: "Import from a link" })
    ).toBeInTheDocument()
    expect(screen.queryByTestId("recipes-empty")).toBeNull()
  })

  it("shows the guided state above the grid below the target", async () => {
    setupWith(4)
    vi.mocked(listFoods).mockResolvedValue({
      items: [mockChickenCurry, mockTofuBowl],
      total: 2,
    })

    renderLibrary()

    expect(
      await screen.findByText(
        "4 of 10 recipes. Plantry needs 10 to plan a varied week."
      )
    ).toBeInTheDocument()
    expect(await screen.findByText("Chicken Curry")).toBeInTheDocument()
  })

  it("hides the guided state at the target", async () => {
    vi.mocked(listFoods).mockResolvedValue({
      items: [mockChickenCurry],
      total: 1,
    })

    renderLibrary()

    expect(await screen.findByText("Chicken Curry")).toBeInTheDocument()
    await vi.waitFor(() => expect(getSetupStatus).toHaveBeenCalled())
    expect(screen.queryByRole("progressbar")).toBeNull()
  })

  it("Show more loads the next page", async () => {
    vi.mocked(listFoods).mockResolvedValue({
      items: [mockChickenCurry],
      total: 30,
    })
    const user = userEvent.setup()

    renderLibrary()
    await user.click(await screen.findByRole("button", { name: "Show more" }))
    expect(listFoods).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 48 })
    )
  })
})
