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

    await screen.findByRole("heading", { name: "The Pantry Archive" })
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

    const searchInput = screen.getByPlaceholderText("Search the archive…")
    await user.type(searchInput, "xyz")

    expect(
      await screen.findByText(
        "Try a different search term or clear the filters."
      )
    ).toBeInTheDocument()
  })

  test("renders ingredient cards with name", async () => {
    vi.mocked(listIngredients).mockResolvedValue({
      items: [mockChickenBreast, mockBrownRice],
      total: 2,
    })
    renderWithRouter(<IngredientList />, "/ingredients")

    expect(await screen.findByText("Chicken breast")).toBeInTheDocument()
    expect(screen.getByText("Brown rice")).toBeInTheDocument()
    const chickenCard = screen.getByTestId(
      `ingredient-card-${mockChickenBreast.id}`
    )
    expect(chickenCard).toHaveTextContent("165")
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

  test("Load more button visible when more items available", async () => {
    vi.mocked(listIngredients).mockResolvedValue({
      items: [mockChickenBreast],
      total: 25,
    })
    renderWithRouter(<IngredientList />, "/ingredients")

    await screen.findByText("Chicken breast")

    expect(
      await screen.findByTestId("ingredients-load-more")
    ).toBeInTheDocument()
  })

  test("clicking Load more increases limit", async () => {
    const user = userEvent.setup()
    vi.mocked(listIngredients).mockResolvedValue({
      items: [mockChickenBreast],
      total: 25,
    })
    renderWithRouter(<IngredientList />, "/ingredients")

    await screen.findByText("Chicken breast")

    await user.click(screen.getByTestId("ingredients-load-more"))

    await waitFor(() => {
      const lastCall = vi.mocked(listIngredients).mock.calls.at(-1)?.[0]
      expect(lastCall?.limit).toBeGreaterThan(24)
    })
  })
})
