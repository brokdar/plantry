import { describe, expect, test, vi, beforeEach } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithRouter } from "@/test/render"
import { mockChickenBreast, mockBrownRice } from "@/test/fixtures"

vi.mock("@/lib/api/ingredients", () => ({
  listIngredients: vi.fn(),
  getIngredient: vi.fn(),
  createIngredient: vi.fn(),
  updateIngredient: vi.fn(),
  deleteIngredient: vi.fn(),
}))

import { listIngredients, deleteIngredient } from "@/lib/api/ingredients"
import { IngredientList } from "./IngredientList"

beforeEach(() => {
  vi.clearAllMocks()
})

describe("IngredientList", () => {
  test("renders loading skeleton while fetching", async () => {
    vi.mocked(listIngredients).mockReturnValue(new Promise(() => {}))
    renderWithRouter(<IngredientList />, "/ingredients")

    await screen.findByRole("heading", { name: "Ingredients" })

    // Table should not be rendered while loading
    expect(screen.queryByRole("table")).not.toBeInTheDocument()
  })

  test("renders empty state when no ingredients", async () => {
    vi.mocked(listIngredients).mockResolvedValue({ items: [], total: 0 })
    renderWithRouter(<IngredientList />, "/ingredients")

    expect(
      await screen.findByText("No ingredients yet. Create your first one!")
    ).toBeInTheDocument()
  })

  test("renders empty search results message", async () => {
    const user = userEvent.setup()
    vi.mocked(listIngredients)
      .mockResolvedValueOnce({ items: [mockChickenBreast], total: 1 })
      .mockResolvedValue({ items: [], total: 0 })

    renderWithRouter(<IngredientList />, "/ingredients")

    await screen.findByText("Chicken breast")

    const searchInput = screen.getByPlaceholderText("Search ingredients...")
    await user.type(searchInput, "xyz")

    expect(await screen.findByText("No ingredients found.")).toBeInTheDocument()
  })

  test("renders ingredient cards with name and kcal", async () => {
    vi.mocked(listIngredients).mockResolvedValue({
      items: [mockChickenBreast, mockBrownRice],
      total: 2,
    })
    renderWithRouter(<IngredientList />, "/ingredients")

    expect(await screen.findByText("Chicken breast")).toBeInTheDocument()
    expect(screen.getByText("Brown rice")).toBeInTheDocument()
    expect(screen.getByText("165 kcal / 100g")).toBeInTheDocument()
    expect(screen.getByText("112 kcal / 100g")).toBeInTheDocument()
  })

  test("shows delete confirmation dialog via card menu", async () => {
    const user = userEvent.setup()
    vi.mocked(listIngredients).mockResolvedValue({
      items: [mockChickenBreast],
      total: 1,
    })
    renderWithRouter(<IngredientList />, "/ingredients")

    await screen.findByText("Chicken breast")

    await user.click(
      screen.getByTestId(`ingredient-card-${mockChickenBreast.id}-menu`)
    )
    await user.click(
      screen.getByTestId(`ingredient-card-${mockChickenBreast.id}-delete`)
    )

    expect(await screen.findByText("Delete ingredient?")).toBeInTheDocument()
  })

  test("calls deleteIngredient on confirm", async () => {
    const user = userEvent.setup()
    vi.mocked(listIngredients).mockResolvedValue({
      items: [mockChickenBreast],
      total: 1,
    })
    vi.mocked(deleteIngredient).mockResolvedValue(undefined)

    renderWithRouter(<IngredientList />, "/ingredients")

    await screen.findByText("Chicken breast")

    await user.click(
      screen.getByTestId(`ingredient-card-${mockChickenBreast.id}-menu`)
    )
    await user.click(
      screen.getByTestId(`ingredient-card-${mockChickenBreast.id}-delete`)
    )

    const dialog = await screen.findByRole("dialog")
    await user.click(within(dialog).getByTestId("confirm-delete"))

    expect(deleteIngredient).toHaveBeenCalledWith(mockChickenBreast.id)
  })

  test("Previous button disabled on first page", async () => {
    vi.mocked(listIngredients).mockResolvedValue({
      items: [mockChickenBreast],
      total: 25,
    })
    renderWithRouter(<IngredientList />, "/ingredients")

    await screen.findByText("Chicken breast")

    const prevButton = screen.getByRole("button", { name: "Previous" })
    expect(prevButton).toBeDisabled()
  })

  test("clicking Next fetches next page", async () => {
    const user = userEvent.setup()
    vi.mocked(listIngredients).mockResolvedValue({
      items: [mockChickenBreast],
      total: 25,
    })
    renderWithRouter(<IngredientList />, "/ingredients")

    await screen.findByText("Chicken breast")

    const nextButton = screen.getByRole("button", { name: "Next" })
    await user.click(nextButton)

    await waitFor(() => {
      expect(listIngredients).toHaveBeenCalledWith(
        expect.objectContaining({ offset: 20 })
      )
    })
  })
})
