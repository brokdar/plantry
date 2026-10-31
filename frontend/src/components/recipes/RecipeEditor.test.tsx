import { describe, it, expect, vi } from "vitest"
import { screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithRouter } from "@/test/render"
import { RecipeEditor } from "./RecipeEditor"
import { mockChickenCurry } from "@/test/fixtures"

vi.mock("@/lib/api/foods", () => ({
  listFoods: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  createFood: vi.fn(),
  updateFood: vi.fn(),
  deleteFood: vi.fn(),
  listPortions: vi.fn().mockResolvedValue({ items: [] }),
  listVariants: vi.fn().mockResolvedValue({ items: [] }),
  createVariant: vi.fn(),
  setDietClass: vi.fn(),
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

import {
  createFood,
  getFoodNutrition,
  setDietClass,
  updateFood,
} from "@/lib/api/foods"

describe("RecipeEditor", () => {
  it("renders empty form in create mode with role selector", async () => {
    renderWithRouter(<RecipeEditor />)

    expect(await screen.findByLabelText(/^name/i)).toBeInTheDocument()
    expect(screen.getAllByText("Main").length).toBeGreaterThan(0) // default role in select
    expect(screen.getByLabelText(/servings/i)).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /save/i })).toBeInTheDocument()
  })

  it("renders populated form in edit mode", async () => {
    renderWithRouter(<RecipeEditor component={mockChickenCurry} />)

    const nameInput = await screen.findByLabelText(/^name/i)
    expect(nameInput).toHaveValue("Chicken Curry")
    expect(screen.getByText("spicy")).toBeInTheDocument()
    expect(screen.getByText("thai")).toBeInTheDocument()
  })

  it("shows instruction fields when component has instructions", async () => {
    renderWithRouter(<RecipeEditor component={mockChickenCurry} />)

    expect(await screen.findByDisplayValue("Cook chicken")).toBeInTheDocument()
    expect(screen.getByDisplayValue("Add curry paste")).toBeInTheDocument()
  })

  it("links Cancel to the recipe detail in edit mode", async () => {
    renderWithRouter(<RecipeEditor component={mockChickenCurry} />)

    expect(await screen.findByRole("link", { name: "Cancel" })).toHaveAttribute(
      "href",
      `/recipes/${mockChickenCurry.id}`
    )
  })

  it("links Cancel to the recipe list in create mode", async () => {
    renderWithRouter(<RecipeEditor />)

    expect(await screen.findByRole("link", { name: "Cancel" })).toHaveAttribute(
      "href",
      "/recipes"
    )
  })

  it("saves the source URL and description", async () => {
    vi.mocked(updateFood).mockResolvedValue(mockChickenCurry)
    const user = userEvent.setup()
    renderWithRouter(<RecipeEditor component={mockChickenCurry} />)

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
      <RecipeEditor
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

  it("F9-AC2: keep days above 4 warns, and keep days and freezable save", async () => {
    vi.mocked(updateFood).mockClear()
    vi.mocked(updateFood).mockResolvedValue(mockChickenCurry)
    const user = userEvent.setup()
    renderWithRouter(<RecipeEditor component={mockChickenCurry} />)

    const keep = await screen.findByLabelText("Keeps (days)")
    expect(keep).toHaveAttribute("placeholder", "Default (3 days)")
    await user.type(keep, "4")
    expect(screen.queryByTestId("recipe-keep-days-warning")).toBeNull()
    await user.clear(keep)
    await user.type(keep, "5")
    expect(screen.getByTestId("recipe-keep-days-warning")).toHaveTextContent(
      "Only go above 4"
    )

    await user.click(screen.getByLabelText("Freezable"))
    await user.click(await screen.findByRole("option", { name: "No" }))
    await user.click(screen.getByRole("button", { name: /save/i }))

    await vi.waitFor(() => expect(updateFood).toHaveBeenCalled())
    expect(vi.mocked(updateFood).mock.calls[0][1]).toEqual(
      expect.objectContaining({ keep_days: 5, freezable: false })
    )
  })

  it("F6: the diet override saves through the diet-class endpoint", async () => {
    vi.mocked(updateFood).mockClear()
    vi.mocked(updateFood).mockResolvedValue(mockChickenCurry)
    vi.mocked(setDietClass).mockResolvedValue(mockChickenCurry)
    const user = userEvent.setup()
    renderWithRouter(<RecipeEditor component={mockChickenCurry} />)

    const diet = await screen.findByRole("combobox", { name: "Diet" })
    expect(diet).toHaveTextContent("With meatauto")
    await user.click(diet)
    await user.click(await screen.findByRole("option", { name: "Pescetarian" }))
    await user.click(screen.getByRole("button", { name: /save/i }))

    await vi.waitFor(() =>
      expect(setDietClass).toHaveBeenCalledWith(
        mockChickenCurry.id,
        "pescetarian"
      )
    )
    expect(vi.mocked(updateFood).mock.calls[0][1]).not.toHaveProperty(
      "diet_class"
    )
  })

  it("an unchanged diet class makes no diet-class call", async () => {
    vi.mocked(setDietClass).mockClear()
    vi.mocked(updateFood).mockClear()
    vi.mocked(updateFood).mockResolvedValue(mockChickenCurry)
    const user = userEvent.setup()
    renderWithRouter(
      <RecipeEditor
        component={{
          ...mockChickenCurry,
          diet_class: "vegan",
          diet_class_source: "user",
        }}
      />
    )

    expect(
      await screen.findByRole("combobox", { name: "Diet" })
    ).toHaveTextContent(/^Vegan$/)
    await user.click(screen.getByRole("button", { name: /save/i }))
    await vi.waitFor(() => expect(updateFood).toHaveBeenCalled())
    expect(setDietClass).not.toHaveBeenCalled()
  })

  it("a new recipe with a diet override sets it after create", async () => {
    vi.mocked(createFood).mockResolvedValue({ ...mockChickenCurry, id: 77 })
    vi.mocked(setDietClass).mockClear()
    vi.mocked(setDietClass).mockResolvedValue(mockChickenCurry)
    const user = userEvent.setup()
    renderWithRouter(<RecipeEditor />)

    await user.type(await screen.findByLabelText(/^name/i), "Lentil soup")
    const diet = screen.getByRole("combobox", { name: "Diet" })
    expect(diet).toHaveTextContent(/^Automatic$/)
    await user.click(diet)
    await user.click(await screen.findByRole("option", { name: "Vegan" }))
    await user.click(screen.getByRole("button", { name: /save/i }))

    await vi.waitFor(() =>
      expect(setDietClass).toHaveBeenCalledWith(77, "vegan")
    )
  })

  it("an empty keep days field saves null (use the default)", async () => {
    vi.mocked(updateFood).mockClear()
    vi.mocked(updateFood).mockResolvedValue(mockChickenCurry)
    const user = userEvent.setup()
    renderWithRouter(
      <RecipeEditor
        component={{ ...mockChickenCurry, keep_days: 2, freezable: true }}
      />
    )

    const keep = await screen.findByLabelText("Keeps (days)")
    expect(keep).toHaveValue(2)
    await user.clear(keep)
    await user.click(screen.getByRole("button", { name: /save/i }))

    await vi.waitFor(() => expect(updateFood).toHaveBeenCalled())
    expect(vi.mocked(updateFood).mock.calls[0][1]).toEqual(
      expect.objectContaining({ keep_days: null, freezable: true })
    )
  })

  it("empty prep and cook minutes save null, not 0", async () => {
    vi.mocked(updateFood).mockClear()
    vi.mocked(updateFood).mockResolvedValue(mockChickenCurry)
    const user = userEvent.setup()
    renderWithRouter(
      <RecipeEditor
        component={{
          ...mockChickenCurry,
          prep_minutes: null,
          cook_minutes: 30,
        }}
      />
    )

    const prep = await screen.findByLabelText("Prep (min)")
    expect(prep).toHaveValue(null)
    await user.clear(screen.getByLabelText("Cook (min)"))
    await user.click(screen.getByRole("button", { name: /save/i }))

    await vi.waitFor(() => expect(updateFood).toHaveBeenCalled())
    expect(vi.mocked(updateFood).mock.calls[0][1]).toEqual(
      expect.objectContaining({ prep_minutes: null, cook_minutes: null })
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
    renderWithRouter(<RecipeEditor component={mockChickenCurry} />)

    const warning = await screen.findByTestId("nutrition-completeness-warning")
    expect(
      within(warning).getByRole("link", { name: "Chicken breast" })
    ).toHaveAttribute("href", "/recipes/ingredients/1/edit")
  })
})
