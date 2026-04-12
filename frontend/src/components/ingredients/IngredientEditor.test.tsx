import { describe, expect, test, vi, beforeEach } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithRouter } from "@/test/render"
import { mockChickenBreast } from "@/test/fixtures"

vi.mock("@/lib/api/ingredients", () => ({
  listIngredients: vi.fn(),
  getIngredient: vi.fn(),
  createIngredient: vi.fn(),
  updateIngredient: vi.fn(),
  deleteIngredient: vi.fn(),
}))

import { createIngredient, updateIngredient } from "@/lib/api/ingredients"
import { ApiError } from "@/lib/api/client"
import { IngredientEditor } from "./IngredientEditor"

beforeEach(() => {
  vi.clearAllMocks()
})

describe("IngredientEditor", () => {
  test("renders create mode with empty fields", async () => {
    renderWithRouter(<IngredientEditor />)

    const nameInput = await screen.findByLabelText("Name")
    expect(nameInput).toHaveValue("")

    const kcalInput = screen.getByLabelText("Calories (kcal)")
    expect(kcalInput).toHaveValue(0)
  })

  test("renders edit mode with pre-filled values", async () => {
    renderWithRouter(<IngredientEditor ingredient={mockChickenBreast} />)

    expect(await screen.findByLabelText("Name")).toHaveValue("Chicken breast")
    expect(screen.getByLabelText("Calories (kcal)")).toHaveValue(165)
    expect(screen.getByLabelText("Protein (g)")).toHaveValue(31)
    expect(screen.getByLabelText("Fat (g)")).toHaveValue(3.6)
  })

  test("shows validation error for empty name", async () => {
    const user = userEvent.setup()
    renderWithRouter(<IngredientEditor />)

    const saveButton = await screen.findByRole("button", { name: "Save" })
    await user.click(saveButton)

    await waitFor(() => {
      expect(createIngredient).not.toHaveBeenCalled()
    })
    // Also verify the error is shown to the user
    expect(
      await screen.findByText(/expected string to have >=1 characters/i)
    ).toBeInTheDocument()
  })

  test("calls createIngredient on submit in create mode", async () => {
    const user = userEvent.setup()
    const onSuccess = vi.fn()
    vi.mocked(createIngredient).mockResolvedValue({
      ...mockChickenBreast,
      id: 2,
      name: "Tofu",
    })

    renderWithRouter(<IngredientEditor onSuccess={onSuccess} />)

    const nameInput = await screen.findByLabelText("Name")
    await user.type(nameInput, "Tofu")

    const kcalInput = screen.getByLabelText("Calories (kcal)")
    await user.clear(kcalInput)
    await user.type(kcalInput, "76")

    const saveButton = screen.getByRole("button", { name: "Save" })
    await user.click(saveButton)

    await waitFor(() => {
      expect(createIngredient).toHaveBeenCalledWith(
        expect.objectContaining({
          name: "Tofu",
          kcal_100g: 76,
        })
      )
    })

    await waitFor(() => {
      expect(onSuccess).toHaveBeenCalled()
    })
  })

  test("calls updateIngredient on submit in edit mode", async () => {
    const user = userEvent.setup()
    const onSuccess = vi.fn()
    vi.mocked(updateIngredient).mockResolvedValue({
      ...mockChickenBreast,
      kcal_100g: 170,
    })

    renderWithRouter(
      <IngredientEditor ingredient={mockChickenBreast} onSuccess={onSuccess} />,
      "/ingredients/1/edit"
    )

    const kcalInput = await screen.findByLabelText("Calories (kcal)")
    await user.clear(kcalInput)
    await user.type(kcalInput, "170")

    const saveButton = screen.getByRole("button", { name: "Save" })
    await user.click(saveButton)

    await waitFor(() => {
      expect(updateIngredient).toHaveBeenCalledWith(
        1,
        expect.objectContaining({
          name: "Chicken breast",
          kcal_100g: 170,
        })
      )
    })

    await waitFor(() => {
      expect(onSuccess).toHaveBeenCalled()
    })
  })

  test("shows server error message", async () => {
    const user = userEvent.setup()
    vi.mocked(createIngredient).mockRejectedValue(
      new ApiError(409, "error.ingredient.duplicate_name")
    )

    renderWithRouter(<IngredientEditor />)

    const nameInput = await screen.findByLabelText("Name")
    await user.type(nameInput, "Chicken breast")

    const saveButton = screen.getByRole("button", { name: "Save" })
    await user.click(saveButton)

    expect(
      await screen.findByText("An ingredient with this name already exists.")
    ).toBeInTheDocument()
  })
})
