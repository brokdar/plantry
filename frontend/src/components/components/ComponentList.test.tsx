import { describe, it, expect, vi } from "vitest"
import { screen } from "@testing-library/react"
import { renderWithRouter } from "@/test/render"
import { ComponentList } from "./ComponentList"
import { mockChickenCurry, mockTofuBowl } from "@/test/fixtures"

vi.mock("@/lib/api/foods", () => ({
  listFoods: vi.fn(),
  deleteFood: vi.fn(),
  getInsights: vi.fn(),
}))

import { listFoods, getInsights } from "@/lib/api/foods"

const noInsights = { forgotten: [], most_cooked: [] }

describe("ComponentList", () => {
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

  it("renders empty state when no components", async () => {
    vi.mocked(listFoods).mockResolvedValue({ items: [], total: 0 })
    vi.mocked(getInsights).mockResolvedValue(noInsights)

    renderWithRouter(<ComponentList />)

    expect(
      await screen.findByText("No recipes yet. Create your first one!")
    ).toBeInTheDocument()
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
