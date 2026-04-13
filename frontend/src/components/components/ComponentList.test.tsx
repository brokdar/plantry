import { describe, it, expect, vi } from "vitest"
import { screen } from "@testing-library/react"
import { renderWithRouter } from "@/test/render"
import { ComponentList } from "./ComponentList"
import { mockChickenCurry, mockTofuBowl } from "@/test/fixtures"

vi.mock("@/lib/api/components", () => ({
  listComponents: vi.fn(),
  deleteComponent: vi.fn(),
}))

import { listComponents } from "@/lib/api/components"

describe("ComponentList", () => {
  it("renders list of components with role badges", async () => {
    vi.mocked(listComponents).mockResolvedValue({
      items: [mockChickenCurry, mockTofuBowl],
      total: 2,
    })

    renderWithRouter(<ComponentList />)

    expect(await screen.findByText("Chicken Curry")).toBeInTheDocument()
    expect(screen.getByText("Tofu Bowl")).toBeInTheDocument()
    expect(screen.getByText("Main")).toBeInTheDocument()
    expect(screen.getByText("Standalone")).toBeInTheDocument()
  })

  it("renders empty state when no components", async () => {
    vi.mocked(listComponents).mockResolvedValue({ items: [], total: 0 })

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

    renderWithRouter(<ComponentList />)

    expect(await screen.findByText("spicy")).toBeInTheDocument()
    expect(screen.getByText("thai")).toBeInTheDocument()
  })
})
