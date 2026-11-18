import { describe, expect, test, vi, beforeEach } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithRouter } from "@/test/render"
import { mockChickenBreast } from "@/test/fixtures"

vi.mock("@/lib/api/foods", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/foods")>()
  return {
    ...actual,
    createFood: vi.fn(),
    updateFood: vi.fn(),
    setNoCook: vi.fn(),
    listPortions: vi.fn().mockResolvedValue({ items: [] }),
  }
})

vi.mock("@/lib/api/lookup", () => ({ lookupFoods: vi.fn() }))

vi.mock("@/components/images/ImageField", () => ({
  ImageField: () => <div data-testid="image-field" />,
}))

import { createFood, setNoCook, updateFood } from "@/lib/api/foods"
import { IngredientEditor } from "./IngredientEditor"

beforeEach(() => {
  vi.clearAllMocks()
})

describe("IngredientEditor planning fields", () => {
  test("shows the derived role in the select and no override", async () => {
    const user = userEvent.setup()
    renderWithRouter(
      <IngredientEditor
        ingredient={{
          ...mockChickenBreast,
          role: "side_protein",
          role_source: "derived",
        }}
      />
    )

    const role = await screen.findByRole("combobox", { name: "Role" })
    expect(role).toHaveTextContent(/^None/)
    await user.click(role)
    expect(
      await screen.findByRole("option", { name: "None (Side (protein))" })
    ).toBeInTheDocument()
    // main and standalone are recipe-only.
    expect(screen.queryByRole("option", { name: "Main" })).toBeNull()
    expect(screen.queryByRole("option", { name: "Standalone" })).toBeNull()
  })

  test("a chosen role is saved with the food", async () => {
    vi.mocked(updateFood).mockResolvedValue(mockChickenBreast)
    const user = userEvent.setup()
    renderWithRouter(<IngredientEditor ingredient={mockChickenBreast} />)

    await user.click(await screen.findByRole("combobox", { name: "Role" }))
    await user.click(await screen.findByRole("option", { name: "Sauce" }))
    await user.click(screen.getByRole("button", { name: "Save" }))

    await waitFor(() =>
      expect(updateFood).toHaveBeenCalledWith(
        mockChickenBreast.id,
        expect.objectContaining({ role: "sauce" })
      )
    )
    expect(vi.mocked(updateFood).mock.calls[0][1]).not.toHaveProperty("no_cook")
    expect(setNoCook).not.toHaveBeenCalled()
  })

  test("the no-cook tri-state saves through its own endpoint", async () => {
    vi.mocked(updateFood).mockResolvedValue(mockChickenBreast)
    vi.mocked(setNoCook).mockResolvedValue(mockChickenBreast)
    const user = userEvent.setup()
    renderWithRouter(<IngredientEditor ingredient={mockChickenBreast} />)

    const noCook = await screen.findByRole("combobox", {
      name: "Needs no cooking",
    })
    expect(noCook).toHaveTextContent(/^Automatic/)
    await user.click(noCook)
    await user.click(await screen.findByRole("option", { name: "Yes" }))
    await user.click(screen.getByRole("button", { name: "Save" }))

    await waitFor(() =>
      expect(setNoCook).toHaveBeenCalledWith(mockChickenBreast.id, true, null)
    )
  })

  test("a new ingredient with a no-cook override sets it after create", async () => {
    vi.mocked(createFood).mockResolvedValue({ ...mockChickenBreast, id: 55 })
    vi.mocked(setNoCook).mockResolvedValue(mockChickenBreast)
    const user = userEvent.setup()
    renderWithRouter(<IngredientEditor />)

    await user.type(await screen.findByLabelText("Name"), "Radish")
    await user.click(screen.getByRole("combobox", { name: "Needs no cooking" }))
    await user.click(await screen.findByRole("option", { name: "No" }))
    await user.click(screen.getByRole("button", { name: "Save" }))

    await waitFor(() => expect(setNoCook).toHaveBeenCalledWith(55, false, null))
  })

  test("an unchanged override makes no no-cook call", async () => {
    vi.mocked(updateFood).mockResolvedValue(mockChickenBreast)
    const user = userEvent.setup()
    renderWithRouter(
      <IngredientEditor
        ingredient={{
          ...mockChickenBreast,
          no_cook: true,
          no_cook_source: "user",
        }}
      />
    )
    expect(
      await screen.findByRole("combobox", { name: "Needs no cooking" })
    ).toHaveTextContent(/^Yes$/)
    await user.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() => expect(updateFood).toHaveBeenCalled())
    expect(setNoCook).not.toHaveBeenCalled()
  })

  test("a serving size saves with the no-cook override", async () => {
    vi.mocked(updateFood).mockResolvedValue(mockChickenBreast)
    vi.mocked(setNoCook).mockResolvedValue(mockChickenBreast)
    const user = userEvent.setup()
    renderWithRouter(<IngredientEditor ingredient={mockChickenBreast} />)

    await user.type(await screen.findByLabelText("Serving size (g)"), "30")
    await user.click(screen.getByRole("button", { name: "Save" }))

    await waitFor(() =>
      expect(setNoCook).toHaveBeenCalledWith(mockChickenBreast.id, null, 30)
    )
    expect(vi.mocked(updateFood).mock.calls[0][1]).not.toHaveProperty(
      "serving_g"
    )
  })

  test("clearing the serving size sends none", async () => {
    vi.mocked(updateFood).mockResolvedValue(mockChickenBreast)
    vi.mocked(setNoCook).mockResolvedValue(mockChickenBreast)
    const user = userEvent.setup()
    renderWithRouter(
      <IngredientEditor ingredient={{ ...mockChickenBreast, serving_g: 40 }} />
    )

    const serving = await screen.findByLabelText("Serving size (g)")
    expect(serving).toHaveValue(40)
    await user.clear(serving)
    await user.click(screen.getByRole("button", { name: "Save" }))

    await waitFor(() =>
      expect(setNoCook).toHaveBeenCalledWith(mockChickenBreast.id, null, null)
    )
  })

  test("a serving size of zero is refused", async () => {
    const user = userEvent.setup()
    renderWithRouter(<IngredientEditor ingredient={mockChickenBreast} />)

    await user.type(await screen.findByLabelText("Serving size (g)"), "0")
    await user.click(screen.getByRole("button", { name: "Save" }))

    await waitFor(() => expect(updateFood).not.toHaveBeenCalled())
    expect(setNoCook).not.toHaveBeenCalled()
  })
})
