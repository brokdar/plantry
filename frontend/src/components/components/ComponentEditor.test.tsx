import { describe, it, expect, vi } from "vitest"
import { screen } from "@testing-library/react"
import { renderWithRouter } from "@/test/render"
import { ComponentEditor } from "./ComponentEditor"
import { mockChickenCurry } from "@/test/fixtures"

vi.mock("@/lib/api/components", () => ({
  createComponent: vi.fn(),
  updateComponent: vi.fn(),
}))

vi.mock("@/lib/api/ingredients", () => ({
  listIngredients: vi.fn().mockResolvedValue({ items: [], total: 0 }),
}))

vi.mock("@/lib/api/portions", () => ({
  listPortions: vi.fn().mockResolvedValue([]),
}))

describe("ComponentEditor", () => {
  it("renders empty form in create mode with role selector", async () => {
    renderWithRouter(<ComponentEditor />)

    expect(await screen.findByLabelText(/^name/i)).toBeInTheDocument()
    expect(screen.getAllByText("Main").length).toBeGreaterThan(0) // default role in select
    expect(screen.getByLabelText(/servings/i)).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /save/i })).toBeInTheDocument()
  })

  it("renders populated form in edit mode", async () => {
    renderWithRouter(<ComponentEditor component={mockChickenCurry} />)

    const nameInput = await screen.findByLabelText(/^name/i)
    expect(nameInput).toHaveValue("Chicken Curry")
    expect(screen.getByText("spicy")).toBeInTheDocument()
    expect(screen.getByText("thai")).toBeInTheDocument()
  })

  it("shows instruction fields when component has instructions", async () => {
    renderWithRouter(<ComponentEditor component={mockChickenCurry} />)

    expect(await screen.findByDisplayValue("Cook chicken")).toBeInTheDocument()
    expect(screen.getByDisplayValue("Add curry paste")).toBeInTheDocument()
  })
})
