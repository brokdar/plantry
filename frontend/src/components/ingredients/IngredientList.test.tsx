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

  test("renders ingredient rows in table", async () => {
    vi.mocked(listIngredients).mockResolvedValue({
      items: [mockChickenBreast, mockBrownRice],
      total: 2,
    })
    renderWithRouter(<IngredientList />, "/ingredients")

    expect(await screen.findByText("Chicken breast")).toBeInTheDocument()
    expect(screen.getByText("Brown rice")).toBeInTheDocument()
    expect(screen.getByText("165")).toBeInTheDocument()
    expect(screen.getByText("112")).toBeInTheDocument()
  })

  test("shows delete confirmation dialog", async () => {
    const user = userEvent.setup()
    vi.mocked(listIngredients).mockResolvedValue({
      items: [mockChickenBreast],
      total: 1,
    })
    renderWithRouter(<IngredientList />, "/ingredients")

    await screen.findByText("Chicken breast")

    const deleteButtons = screen.getAllByRole("button", { name: "Delete" })
    await user.click(deleteButtons[0])

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

    const deleteButtons = screen.getAllByRole("button", { name: "Delete" })
    await user.click(deleteButtons[0])

    const dialog = await screen.findByRole("dialog")
    const confirmButton = within(dialog).getByRole("button", {
      name: "Delete",
    })
    await user.click(confirmButton)

    expect(deleteIngredient).toHaveBeenCalledWith(1)
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
