import React from "react"
import { describe, expect, test, vi, beforeEach } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithRouter } from "@/test/render"
import { PortionsEditor } from "./PortionsEditor"

vi.mock("@/lib/api/foods", () => ({
  listPortions: vi.fn(),
  upsertPortion: vi.fn(),
  deletePortion: vi.fn(),
}))

import { listPortions, upsertPortion, deletePortion } from "@/lib/api/foods"

beforeEach(() => {
  vi.clearAllMocks()
})

describe("PortionsEditor", () => {
  test("renders existing portions", async () => {
    vi.mocked(listPortions).mockResolvedValue({
      items: [
        { food_id: 1, unit: "cup", grams: 240 },
        { food_id: 1, unit: "tbsp", grams: 15 },
      ],
    })

    renderWithRouter(<PortionsEditor mode="bound" foodId={1} />)

    // UnitLabel shows canonical key + localized name; assert both rows landed.
    expect(await screen.findByText("240 g")).toBeInTheDocument()
    expect(screen.getByText("15 g")).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: /delete cup/i })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: /delete tbsp/i })
    ).toBeInTheDocument()
  })

  test("adds a new portion via the unit picker", async () => {
    const user = userEvent.setup()
    vi.mocked(listPortions).mockResolvedValue({ items: [] })
    vi.mocked(upsertPortion).mockResolvedValue({
      food_id: 1,
      unit: "slice",
      grams: 30,
    })

    renderWithRouter(<PortionsEditor mode="bound" foodId={1} />)

    await screen.findByTestId("portion-unit")

    await user.click(screen.getByTestId("portion-unit"))
    await user.click(await screen.findByTestId("unit-option-slice"))

    const gramsInput = screen.getByTestId("portion-grams")
    await user.type(gramsInput, "30")

    await user.click(screen.getByRole("button", { name: /add portion/i }))

    await waitFor(() => {
      expect(upsertPortion).toHaveBeenCalledWith(1, {
        unit: "slice",
        grams: 30,
      })
    })
  })

  test("hides units that already have a portion so duplicates can't be added", async () => {
    const user = userEvent.setup()
    vi.mocked(listPortions).mockResolvedValue({
      items: [{ food_id: 1, unit: "tbsp", grams: 15 }],
    })

    renderWithRouter(<PortionsEditor mode="bound" foodId={1} />)

    await screen.findByTestId("portion-unit")
    await user.click(screen.getByTestId("portion-unit"))

    // tbsp is already in use → not offered in the picker.
    expect(screen.queryByTestId("unit-option-tbsp")).not.toBeInTheDocument()
    // Other units remain available.
    expect(await screen.findByTestId("unit-option-cup")).toBeInTheDocument()
  })

  test("deletes a portion", async () => {
    const user = userEvent.setup()
    vi.mocked(listPortions).mockResolvedValue({
      items: [{ food_id: 1, unit: "cup", grams: 240 }],
    })
    vi.mocked(deletePortion).mockResolvedValue(undefined)

    renderWithRouter(<PortionsEditor mode="bound" foodId={1} />)

    const deleteButton = await screen.findByRole("button", {
      name: /delete cup/i,
    })
    await user.click(deleteButton)

    await waitFor(() => {
      expect(deletePortion).toHaveBeenCalledWith(1, "cup")
    })
  })

  test("add button stays disabled until both fields are filled", async () => {
    const user = userEvent.setup()
    vi.mocked(listPortions).mockResolvedValue({ items: [] })

    renderWithRouter(<PortionsEditor mode="bound" foodId={1} />)

    await screen.findByTestId("portion-unit")

    const addButton = screen.getByRole("button", { name: /add portion/i })
    expect(addButton).toBeDisabled()

    await user.type(screen.getByTestId("portion-grams"), "30")
    expect(addButton).toBeDisabled()
  })

  test("staged mode: add and delete portions via onChange without API calls", async () => {
    const user = userEvent.setup()
    const changes: Array<{ unit: string; grams: number }[]> = []

    function Harness() {
      const [portions, setPortions] = React.useState<
        { unit: string; grams: number }[]
      >([])
      return (
        <PortionsEditor
          mode="staged"
          portions={portions}
          onChange={(next) => {
            changes.push(next)
            setPortions(next)
          }}
        />
      )
    }

    renderWithRouter(<Harness />)

    await user.click(await screen.findByTestId("portion-unit"))
    await user.click(await screen.findByTestId("unit-option-slice"))
    await user.type(screen.getByTestId("portion-grams"), "25")
    await user.click(screen.getByRole("button", { name: /add portion/i }))

    expect(changes.at(-1)).toEqual([{ unit: "slice", grams: 25 }])
    expect(upsertPortion).not.toHaveBeenCalled()
    expect(await screen.findByText("25 g")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: /delete slice/i }))
    expect(changes.at(-1)).toEqual([])
    expect(deletePortion).not.toHaveBeenCalled()
  })
})
