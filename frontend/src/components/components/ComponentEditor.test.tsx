import { describe, it, expect, vi } from "vitest"
import { screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithRouter } from "@/test/render"
import { ComponentEditor } from "./ComponentEditor"
import { mockChickenCurry } from "@/test/fixtures"

vi.mock("@/lib/api/foods", () => ({
  listFoods: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  createFood: vi.fn(),
  updateFood: vi.fn(),
  deleteFood: vi.fn(),
  listPortions: vi.fn().mockResolvedValue({ items: [] }),
  listVariants: vi.fn().mockResolvedValue({ items: [] }),
  createVariant: vi.fn(),
  getFoodNutrition: vi.fn().mockResolvedValue({
    kcal: 0,
    protein: 0,
    fat: 0,
    carbs: 0,
    fiber: 0,
    sodium: 0,
    complete: true,
    missing: [],
  }),
}))

import { getFoodNutrition, updateFood } from "@/lib/api/foods"

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

  it("links Cancel to the recipe detail in edit mode", async () => {
    renderWithRouter(<ComponentEditor component={mockChickenCurry} />)

    expect(await screen.findByRole("link", { name: "Cancel" })).toHaveAttribute(
      "href",
      `/components/${mockChickenCurry.id}`
    )
  })

  it("links Cancel to the recipe list in create mode", async () => {
    renderWithRouter(<ComponentEditor />)

    expect(await screen.findByRole("link", { name: "Cancel" })).toHaveAttribute(
      "href",
      "/components"
    )
  })

  it("saves the source URL and description", async () => {
    vi.mocked(updateFood).mockResolvedValue(mockChickenCurry)
    const user = userEvent.setup()
    renderWithRouter(<ComponentEditor component={mockChickenCurry} />)

    await user.type(
      await screen.findByLabelText("Source URL"),
      "https://example.com/curry"
    )
    await user.type(
      screen.getByLabelText("Description"),
      "Imported blurb about the dish."
    )
    await user.click(screen.getByRole("button", { name: /save/i }))

    await vi.waitFor(() => expect(updateFood).toHaveBeenCalled())
    expect(vi.mocked(updateFood).mock.calls[0]).toEqual([
      mockChickenCurry.id,
      expect.objectContaining({
        source_url: "https://example.com/curry",
        description: "Imported blurb about the dish.",
      }),
    ])
  })

  it("sends empty strings to clear the source URL and description", async () => {
    vi.mocked(updateFood).mockClear()
    vi.mocked(updateFood).mockResolvedValue(mockChickenCurry)
    const user = userEvent.setup()
    renderWithRouter(
      <ComponentEditor
        component={{
          ...mockChickenCurry,
          source_url: "https://example.com/old",
          description: "Old blurb",
        }}
      />
    )

    await user.clear(await screen.findByLabelText("Source URL"))
    await user.clear(screen.getByLabelText("Description"))
    await user.click(screen.getByRole("button", { name: /save/i }))

    await vi.waitFor(() => expect(updateFood).toHaveBeenCalled())
    expect(vi.mocked(updateFood).mock.calls[0][1]).toEqual(
      expect.objectContaining({ source_url: "", description: "" })
    )
  })

  it("shows the completeness warning for incomplete saved nutrition", async () => {
    vi.mocked(getFoodNutrition).mockResolvedValueOnce({
      kcal: 0,
      protein: 0,
      fat: 0,
      carbs: 0,
      fiber: 0,
      sodium: 0,
      complete: false,
      missing: [{ id: 1, name: "Chicken breast" }],
    })
    renderWithRouter(<ComponentEditor component={mockChickenCurry} />)

    const warning = await screen.findByTestId("nutrition-completeness-warning")
    expect(
      within(warning).getByRole("link", { name: "Chicken breast" })
    ).toHaveAttribute("href", "/ingredients/1/edit")
  })
})
