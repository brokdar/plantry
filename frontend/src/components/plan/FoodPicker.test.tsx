import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { Food } from "@/lib/api/foods"
import i18n from "@/lib/i18n"
import {
  mockBrownRice,
  mockChickenCurry,
  mockChili,
  mockPlate,
} from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/foods", () => ({
  listFoods: vi.fn(),
  getFood: vi.fn(),
}))
vi.mock("@/lib/api/plates", () => ({
  createPlate: vi.fn(),
  addPlateComponent: vi.fn(),
  updatePlateComponent: vi.fn(),
}))
vi.mock("@/lib/toast", () => ({ toast: vi.fn(), toastError: vi.fn() }))

import { getFood, listFoods } from "@/lib/api/foods"
import {
  addPlateComponent,
  createPlate,
  updatePlateComponent,
} from "@/lib/api/plates"

import { FoodPicker, type PickerTarget } from "./FoodPicker"

const eaten: Food = { ...mockChili, last_eaten: "2026-10-20" }

beforeEach(async () => {
  vi.clearAllMocks()
  await i18n.changeLanguage("en")
  vi.mocked(listFoods).mockImplementation(async (params) => {
    if (params?.kind === "leaf") return { items: [mockBrownRice], total: 1 }
    return { items: [mockChili, mockChickenCurry], total: 2 }
  })
  vi.mocked(getFood).mockResolvedValue(mockBrownRice)
  vi.mocked(createPlate).mockResolvedValue(mockPlate())
  vi.mocked(addPlateComponent).mockResolvedValue(mockPlate())
  vi.mocked(updatePlateComponent).mockResolvedValue(mockPlate())
})

function renderPicker(target: PickerTarget) {
  const onDone = vi.fn()
  const onCancel = vi.fn()
  renderWithRouter(
    <FoodPicker target={target} onDone={onDone} onCancel={onCancel} />
  )
  return { onDone, onCancel }
}

const row = (name: string) =>
  screen.findByRole("button", { name: new RegExp(`^${name}`) })

describe("FoodPicker", () => {
  // F4-AC7: two staged foods, one POST /api/plates.
  it("stages a recipe and an ingredient and creates the plate once", async () => {
    const { onDone } = renderPicker({
      kind: "create",
      date: "2026-11-01",
      slotId: 3,
    })
    await userEvent.click(await row("Chili"))
    await userEvent.click(screen.getByRole("tab", { name: "Ingredients" }))
    await userEvent.click(await row("Brown rice"))

    const staged = screen.getByTestId("food-picker-staged")
    expect(
      within(staged).getAllByTestId("food-picker-staged-row")
    ).toHaveLength(2)
    await userEvent.click(
      within(staged).getByRole("button", {
        name: "Increase Portions of Chili per eater",
      })
    )
    await userEvent.click(screen.getByRole("button", { name: "Add 2 foods" }))

    await waitFor(() => expect(onDone).toHaveBeenCalled())
    expect(createPlate).toHaveBeenCalledTimes(1)
    expect(createPlate).toHaveBeenCalledWith({
      date: "2026-11-01",
      slot_id: 3,
      components: [
        { food_id: 42, portions: 1.25 },
        { food_id: mockBrownRice.id, amount: 100, unit: "g" },
      ],
    })
  })

  it("unstages a food tapped twice", async () => {
    renderPicker({ kind: "create", date: "2026-11-01", slotId: 3 })
    const chili = await row("Chili")
    await userEvent.click(chili)
    expect(chili).toHaveAttribute("aria-pressed", "true")
    await userEvent.click(chili)
    expect(chili).toHaveAttribute("aria-pressed", "false")
    expect(screen.getByTestId("food-picker-commit")).toBeDisabled()
  })

  it("adds staged foods to an existing plate", async () => {
    const { onDone } = renderPicker({ kind: "add", plateId: 5 })
    await userEvent.click(await row("Chicken Curry"))
    await userEvent.click(screen.getByRole("button", { name: "Add 1 food" }))
    await waitFor(() => expect(onDone).toHaveBeenCalled())
    expect(addPlateComponent).toHaveBeenCalledWith(5, {
      food_id: mockChickenCurry.id,
      portions: 1,
    })
  })

  it("a retry after a failed add does not add the saved food again", async () => {
    vi.mocked(addPlateComponent)
      .mockResolvedValueOnce(mockPlate())
      .mockRejectedValueOnce(new Error("offline"))
    const { onDone } = renderPicker({ kind: "add", plateId: 5 })
    await userEvent.click(await row("Chili"))
    await userEvent.click(await row("Chicken Curry"))
    await userEvent.click(screen.getByRole("button", { name: "Add 2 foods" }))
    await waitFor(() => expect(addPlateComponent).toHaveBeenCalledTimes(2))
    expect(onDone).not.toHaveBeenCalled()
    // Only the food that failed stays staged.
    expect(
      await screen.findByRole("button", { name: "Add 1 food" })
    ).toBeEnabled()
    await userEvent.click(screen.getByRole("button", { name: "Add 1 food" }))
    await waitFor(() => expect(onDone).toHaveBeenCalled())
    expect(
      vi.mocked(addPlateComponent).mock.calls.map((c) => c[1].food_id)
    ).toEqual([42, mockChickenCurry.id, mockChickenCurry.id])
  })

  it("swaps a component at the first tap", async () => {
    const { onDone } = renderPicker({
      kind: "swap",
      plateId: 5,
      componentId: 9,
      name: "Chili",
    })
    await userEvent.click(await row("Chicken Curry"))
    await waitFor(() => expect(onDone).toHaveBeenCalled())
    expect(updatePlateComponent).toHaveBeenCalledWith(5, 9, {
      food_id: mockChickenCurry.id,
    })
    expect(screen.queryByTestId("food-picker-commit")).not.toBeInTheDocument()
  })

  it("filters recipes by role chip and searches", async () => {
    renderPicker({ kind: "create", date: "2026-11-01", slotId: 3 })
    await row("Chili")
    await userEvent.click(screen.getByRole("button", { name: "Dessert" }))
    await waitFor(() =>
      expect(listFoods).toHaveBeenLastCalledWith(
        expect.objectContaining({ kind: "composed", role: "dessert" })
      )
    )
    await userEvent.type(screen.getByTestId("food-picker-search"), "chi")
    await waitFor(() =>
      expect(listFoods).toHaveBeenLastCalledWith(
        expect.objectContaining({ search: "chi", role: "dessert" })
      )
    )
  })

  it("lists recently eaten recipes under Recent and favorites under Favorites", async () => {
    vi.mocked(listFoods).mockImplementation(async (params) =>
      params?.sort === "last_eaten"
        ? { items: [eaten, mockChickenCurry], total: 2 }
        : { items: [mockChickenCurry], total: 1 }
    )
    renderPicker({ kind: "create", date: "2026-11-01", slotId: 3 })
    await userEvent.click(await screen.findByRole("tab", { name: "Recent" }))
    expect(await row("Chili")).toBeInTheDocument()
    // Never eaten: not recent.
    expect(
      screen.queryByRole("button", { name: /^Chicken Curry/ })
    ).not.toBeInTheDocument()
    expect(listFoods).toHaveBeenLastCalledWith(
      expect.objectContaining({ sort: "last_eaten", order: "desc" })
    )
    await userEvent.click(screen.getByRole("tab", { name: "Favorites" }))
    await waitFor(() =>
      expect(listFoods).toHaveBeenLastCalledWith(
        expect.objectContaining({ favorite: 1 })
      )
    )
  })
})
