import { describe, expect, test, vi, beforeEach } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithRouter } from "@/test/render"
import { PortionsEditor } from "./PortionsEditor"

vi.mock("@/lib/api/portions", () => ({
  listPortions: vi.fn(),
  upsertPortion: vi.fn(),
  deletePortion: vi.fn(),
}))

import { listPortions, upsertPortion, deletePortion } from "@/lib/api/portions"

beforeEach(() => {
  vi.clearAllMocks()
})

describe("PortionsEditor", () => {
  test("renders existing portions", async () => {
    vi.mocked(listPortions).mockResolvedValue([
      { ingredient_id: 1, unit: "cup", grams: 240 },
      { ingredient_id: 1, unit: "tbsp", grams: 15 },
    ])

    renderWithRouter(<PortionsEditor ingredientId={1} />)

    expect(await screen.findByText("cup")).toBeInTheDocument()
    expect(screen.getByText("240g")).toBeInTheDocument()
    expect(screen.getByText("tbsp")).toBeInTheDocument()
    expect(screen.getByText("15g")).toBeInTheDocument()
  })

  test("adds a new portion", async () => {
    const user = userEvent.setup()
    vi.mocked(listPortions).mockResolvedValue([])
    vi.mocked(upsertPortion).mockResolvedValue(undefined)

    renderWithRouter(<PortionsEditor ingredientId={1} />)

    await screen.findByPlaceholderText(/cup, tbsp, slice/i)

    const unitInput = screen.getByPlaceholderText(/cup, tbsp, slice/i)
    await user.type(unitInput, "slice")

    const gramsInput = screen.getByRole("spinbutton")
    await user.type(gramsInput, "30")

    const addButton = screen.getByRole("button", { name: /add portion/i })
    await user.click(addButton)

    await waitFor(() => {
      expect(upsertPortion).toHaveBeenCalledWith(1, {
        unit: "slice",
        grams: 30,
      })
    })
  })

  test("deletes a portion", async () => {
    const user = userEvent.setup()
    vi.mocked(listPortions).mockResolvedValue([
      { ingredient_id: 1, unit: "cup", grams: 240 },
    ])
    vi.mocked(deletePortion).mockResolvedValue(undefined)

    renderWithRouter(<PortionsEditor ingredientId={1} />)

    await screen.findByText("cup")

    const deleteButton = screen.getByRole("button", { name: /delete cup/i })
    await user.click(deleteButton)

    await waitFor(() => {
      expect(deletePortion).toHaveBeenCalledWith(1, "cup")
    })
  })

  test("does not submit with empty unit", async () => {
    const user = userEvent.setup()
    vi.mocked(listPortions).mockResolvedValue([])

    renderWithRouter(<PortionsEditor ingredientId={1} />)
    await screen.findByPlaceholderText(/cup, tbsp, slice/i)

    // Only fill grams, leave unit empty
    const gramsInput = screen.getByRole("spinbutton")
    await user.type(gramsInput, "30")

    const addButton = screen.getByRole("button", { name: /add portion/i })
    // Button should be disabled when unit is empty
    expect(addButton).toBeDisabled()
  })

  test("does not submit with zero grams", async () => {
    const user = userEvent.setup()
    vi.mocked(listPortions).mockResolvedValue([])
    vi.mocked(upsertPortion).mockResolvedValue(undefined)

    renderWithRouter(<PortionsEditor ingredientId={1} />)
    await screen.findByPlaceholderText(/cup, tbsp, slice/i)

    const unitInput = screen.getByPlaceholderText(/cup, tbsp, slice/i)
    await user.type(unitInput, "slice")

    const gramsInput = screen.getByRole("spinbutton")
    await user.type(gramsInput, "0")

    const addButton = screen.getByRole("button", { name: /add portion/i })
    await user.click(addButton)

    expect(upsertPortion).not.toHaveBeenCalled()
  })
})
