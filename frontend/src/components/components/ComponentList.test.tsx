import { beforeEach, describe, it, expect, vi } from "vitest"
import { screen, within } from "@testing-library/react"
import { renderWithRouter } from "@/test/render"
import { ComponentList } from "./ComponentList"
import { mockChickenCurry, mockTofuBowl } from "@/test/fixtures"

vi.mock("@/lib/api/foods", () => ({
  listFoods: vi.fn(),
  deleteFood: vi.fn(),
  getInsights: vi.fn(),
}))

vi.mock("@/lib/api/setup", () => ({
  getSetupStatus: vi.fn(),
  completeSetup: vi.fn(),
}))

import { listFoods, getInsights } from "@/lib/api/foods"
import { getSetupStatus } from "@/lib/api/setup"

const noInsights = { forgotten: [], most_cooked: [] }

function setupWith(recipe_count: number) {
  vi.mocked(getSetupStatus).mockResolvedValue({
    completed: true,
    recipe_count,
    recipe_target: 10,
  })
}

describe("ComponentList", () => {
  beforeEach(() => {
    localStorage.clear()
    setupWith(10)
  })

  it("renders list of components with role badges", async () => {
    vi.mocked(listFoods).mockResolvedValue({
      items: [mockChickenCurry, mockTofuBowl],
      total: 2,
    })
    vi.mocked(getInsights).mockResolvedValue(noInsights)

    renderWithRouter(<ComponentList />)

    expect(await screen.findByText("Chicken Curry")).toBeInTheDocument()
    expect(screen.getByText("Tofu Bowl")).toBeInTheDocument()
    // Role labels also appear as filter chips — pick the card scope.
    const mainCard = screen.getByTestId(`component-card-${mockChickenCurry.id}`)
    const standaloneCard = screen.getByTestId(
      `component-card-${mockTofuBowl.id}`
    )
    expect(mainCard).toHaveTextContent("Main")
    expect(standaloneCard).toHaveTextContent("Standalone")
  })

  it("links to Ingredients and Import from the header", async () => {
    vi.mocked(listFoods).mockResolvedValue({ items: [], total: 0 })
    vi.mocked(getInsights).mockResolvedValue(noInsights)

    renderWithRouter(<ComponentList />)

    const sections = await screen.findByRole("navigation", {
      name: "Recipe sections",
    })
    expect(
      within(sections).getByRole("link", { name: "Ingredients" })
    ).toHaveAttribute("href", "/ingredients")
    expect(
      within(sections).getByRole("link", { name: "Import" })
    ).toHaveAttribute("href", "/import")
  })

  it("an empty library shows the onboarding cards in place of the empty grid", async () => {
    setupWith(0)
    vi.mocked(listFoods).mockResolvedValue({ items: [], total: 0 })
    vi.mocked(getInsights).mockResolvedValue(noInsights)

    renderWithRouter(<ComponentList />)

    expect(
      await screen.findByText(
        "0 of 10 recipes. Plantry needs 10 to plan a varied week."
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole("heading", { name: "Import from a link" })
    ).toBeInTheDocument()
    expect(screen.queryByTestId("component-create-tile")).toBeNull()
  })

  it("shows the guided state above the grid below the target", async () => {
    setupWith(4)
    vi.mocked(listFoods).mockResolvedValue({
      items: [mockChickenCurry, mockTofuBowl],
      total: 2,
    })
    vi.mocked(getInsights).mockResolvedValue(noInsights)

    renderWithRouter(<ComponentList />)

    expect(
      await screen.findByText(
        "4 of 10 recipes. Plantry needs 10 to plan a varied week."
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole("heading", { name: "Create manually" })
    ).toBeInTheDocument()
    expect(await screen.findByText("Chicken Curry")).toBeInTheDocument()
  })

  it("hides the guided state at the target", async () => {
    vi.mocked(listFoods).mockResolvedValue({
      items: [mockChickenCurry],
      total: 1,
    })
    vi.mocked(getInsights).mockResolvedValue(noInsights)

    renderWithRouter(<ComponentList />)

    expect(await screen.findByText("Chicken Curry")).toBeInTheDocument()
    await vi.waitFor(() => expect(getSetupStatus).toHaveBeenCalled())
    expect(screen.queryByRole("progressbar")).toBeNull()
    expect(
      screen.queryByRole("heading", { name: "Import from a link" })
    ).toBeNull()
  })

  it("the header create button is the default primary button", async () => {
    vi.mocked(listFoods).mockResolvedValue({ items: [], total: 0 })
    vi.mocked(getInsights).mockResolvedValue(noInsights)

    renderWithRouter(<ComponentList />)

    const create = await screen.findByRole("link", { name: "New recipe" })
    expect(create).toHaveAttribute("href", "/components/new")
    expect(create).toHaveAttribute("data-variant", "default")
    expect(create.className).not.toMatch(/gradient-primary|editorial-shadow/)
  })

  it("shows tags as badges", async () => {
    vi.mocked(listFoods).mockResolvedValue({
      items: [mockChickenCurry],
      total: 1,
    })
    vi.mocked(getInsights).mockResolvedValue(noInsights)

    renderWithRouter(<ComponentList />)

    const card = await screen.findByTestId(
      `component-card-${mockChickenCurry.id}`
    )
    expect(card).toHaveTextContent("spicy")
    expect(card).toHaveTextContent("thai")
  })

  it("renders Forgotten and Most cooked badges from insights", async () => {
    vi.mocked(listFoods).mockResolvedValue({
      items: [mockChickenCurry, mockTofuBowl],
      total: 2,
    })
    vi.mocked(getInsights).mockResolvedValue({
      forgotten: [
        {
          id: mockChickenCurry.id,
          name: mockChickenCurry.name,
          role: mockChickenCurry.role,
          image_path: null,
          cook_count: 0,
          last_cooked_at: null,
        },
      ],
      most_cooked: [
        {
          id: mockTofuBowl.id,
          name: mockTofuBowl.name,
          role: mockTofuBowl.role,
          image_path: null,
          cook_count: 4,
          last_cooked_at: "2026-04-15T00:00:00Z",
        },
      ],
    })

    renderWithRouter(<ComponentList />)

    expect(
      await screen.findByTestId(`badge-forgotten-${mockChickenCurry.id}`)
    ).toBeInTheDocument()
    expect(
      screen.getByTestId(`badge-most-cooked-${mockTofuBowl.id}`)
    ).toBeInTheDocument()
  })
})
