import { describe, it, expect, vi } from "vitest"
import { screen } from "@testing-library/react"
import { renderWithRouter } from "@/test/render"
import { ComponentList } from "./ComponentList"
import { mockChickenCurry, mockTofuBowl } from "@/test/fixtures"

vi.mock("@/lib/api/components", () => ({
  listComponents: vi.fn(),
  deleteComponent: vi.fn(),
  getInsights: vi.fn(),
}))

import { listComponents, getInsights } from "@/lib/api/components"

const noInsights = { forgotten: [], most_cooked: [] }

describe("ComponentList", () => {
  it("renders list of components with role badges", async () => {
    vi.mocked(listComponents).mockResolvedValue({
      items: [mockChickenCurry, mockTofuBowl],
      total: 2,
    })
    vi.mocked(getInsights).mockResolvedValue(noInsights)

    renderWithRouter(<ComponentList />)

    expect(await screen.findByText("Chicken Curry")).toBeInTheDocument()
    expect(screen.getByText("Tofu Bowl")).toBeInTheDocument()
    expect(screen.getByText("Main")).toBeInTheDocument()
    expect(screen.getByText("Standalone")).toBeInTheDocument()
  })

  it("renders empty state when no components", async () => {
    vi.mocked(listComponents).mockResolvedValue({ items: [], total: 0 })
    vi.mocked(getInsights).mockResolvedValue(noInsights)

    renderWithRouter(<ComponentList />)

    expect(
      await screen.findByText("No components yet. Create your first one!")
    ).toBeInTheDocument()
  })

  it("shows tags as badges", async () => {
    vi.mocked(listComponents).mockResolvedValue({
      items: [mockChickenCurry],
      total: 1,
    })
    vi.mocked(getInsights).mockResolvedValue(noInsights)

    renderWithRouter(<ComponentList />)

    expect(await screen.findByText("spicy")).toBeInTheDocument()
    expect(screen.getByText("thai")).toBeInTheDocument()
  })

  it("renders Forgotten and Most cooked badges from insights", async () => {
    vi.mocked(listComponents).mockResolvedValue({
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
