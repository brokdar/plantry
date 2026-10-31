import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { mockRoutines, mockTimeSlots } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/presets", () => ({
  getPresets: vi.fn(),
  getPreset: vi.fn(),
  getKnownTags: vi.fn(),
  createPreset: vi.fn(),
  updatePreset: vi.fn(),
  patchPreset: vi.fn(),
  duplicatePreset: vi.fn(),
  deletePreset: vi.fn(),
}))
vi.mock("@/lib/api/slots", () => ({ listTimeSlots: vi.fn() }))
vi.mock("@/lib/api/foods", () => ({ listFoods: vi.fn() }))

import { listFoods } from "@/lib/api/foods"
import {
  createPreset,
  deletePreset,
  duplicatePreset,
  getKnownTags,
  getPreset,
  getPresets,
  updatePreset,
} from "@/lib/api/presets"
import { listTimeSlots } from "@/lib/api/slots"

import { RoutinesEditor } from "./RoutinesEditor"

const oats = mockRoutines[0]!
const soup = mockRoutines[1]!
const foods = [
  { id: 41, kind: "leaf", name: "Rolled oats" },
  { id: 42, kind: "composed", name: "Lentil soup" },
]

function card(name: string) {
  return screen
    .getAllByTestId("routine-card")
    .find((c) => within(c).queryByText(name))!
}

async function openMenu(
  user: ReturnType<typeof userEvent.setup>,
  name: string
) {
  await screen.findByText(name)
  await user.click(
    within(card(name)).getByRole("button", { name: `Actions for ${name}` })
  )
}

describe("RoutinesEditor (F10-AC1)", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getPresets).mockResolvedValue({
      items: mockRoutines,
      total: 2,
      known_tags: [],
    })
    vi.mocked(getKnownTags).mockResolvedValue({ items: [] })
    vi.mocked(listTimeSlots).mockResolvedValue({ items: mockTimeSlots })
    vi.mocked(listFoods).mockResolvedValue({
      items: foods,
      total: foods.length,
    } as unknown as Awaited<ReturnType<typeof listFoods>>)
  })

  it("lists routines with their meals and foods", async () => {
    renderWithRouter(<RoutinesEditor />)
    expect(await screen.findByText("Overnight oats")).toBeInTheDocument()
    expect(
      await within(card("Overnight oats")).findByText("Rolled oats")
    ).toBeInTheDocument()
    expect(
      within(card("Overnight oats")).getByText("Breakfast")
    ).toBeInTheDocument()
    expect(
      within(card("Bread and soup")).getByText("Dinner")
    ).toBeInTheDocument()
  })

  it("creates a routine from a name, a meal and a food", async () => {
    vi.mocked(createPreset).mockResolvedValue({ ...soup, id: 9 })
    const user = userEvent.setup()
    renderWithRouter(<RoutinesEditor />)
    await screen.findByText("Overnight oats")

    await user.click(screen.getByRole("button", { name: "New routine" }))
    const sheet = await screen.findByTestId("routine-sheet")
    const create = within(sheet).getByRole("button", {
      name: "Create routine",
    })
    expect(create).toBeDisabled()

    await user.type(within(sheet).getByLabelText("Name"), "Soup night")
    await user.selectOptions(within(sheet).getByLabelText("Meal"), "3")
    await user.click(within(sheet).getByRole("button", { name: "Add food" }))
    await user.click(
      await within(sheet).findByRole("button", { name: "Lentil soup" })
    )
    await user.click(create)

    await waitFor(() =>
      expect(createPreset).toHaveBeenCalledWith({
        name: "Soup night",
        tags: [],
        plates: [
          {
            slot_id: 3,
            components: [
              { food_id: 42, portions: 1, amount: null, unit: null },
            ],
          },
        ],
      })
    )
    await waitFor(() =>
      expect(screen.queryByTestId("routine-sheet")).not.toBeInTheDocument()
    )
  })

  it("edits a routine's meal and saves the whole composition", async () => {
    vi.mocked(getPreset).mockResolvedValue(soup)
    vi.mocked(updatePreset).mockResolvedValue(soup)
    const user = userEvent.setup()
    renderWithRouter(<RoutinesEditor />)
    await user.click(
      await screen.findByRole("button", { name: /^Bread and soup/ })
    )
    const sheet = await screen.findByTestId("routine-sheet")

    await user.click(
      await within(sheet).findByRole("button", {
        name: "Increase Portions of Lentil soup",
      })
    )
    await waitFor(() =>
      expect(updatePreset).toHaveBeenCalledWith(8, {
        plates: [
          {
            slot_id: 3,
            components: [
              { food_id: 42, portions: 1.25, amount: null, unit: null },
            ],
          },
        ],
      })
    )
  })

  it("renames a routine", async () => {
    vi.mocked(updatePreset).mockResolvedValue({ ...oats, name: "Oats" })
    const user = userEvent.setup()
    renderWithRouter(<RoutinesEditor />)
    await openMenu(user, "Overnight oats")
    await user.click(await screen.findByRole("menuitem", { name: "Rename" }))
    const field = await screen.findByLabelText("Name")
    await user.clear(field)
    await user.type(field, "Oats{Enter}")
    await waitFor(() =>
      expect(updatePreset).toHaveBeenCalledWith(7, { name: "Oats" })
    )
  })

  it("duplicates a routine", async () => {
    vi.mocked(duplicatePreset).mockResolvedValue({
      ...oats,
      id: 10,
      name: "Overnight oats (copy)",
    })
    const user = userEvent.setup()
    renderWithRouter(<RoutinesEditor />)
    await openMenu(user, "Overnight oats")
    await user.click(await screen.findByRole("menuitem", { name: "Duplicate" }))
    await waitFor(() => expect(duplicatePreset).toHaveBeenCalledWith(7))
  })

  it("deletes a routine after confirming", async () => {
    vi.mocked(deletePreset).mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderWithRouter(<RoutinesEditor />)
    await openMenu(user, "Bread and soup")
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }))
    const dialog = await screen.findByRole("dialog", {
      name: "Delete “Bread and soup”?",
    })
    await user.click(within(dialog).getByRole("button", { name: "Delete" }))
    await waitFor(() => expect(deletePreset).toHaveBeenCalledWith(8))
  })

  it("teaches the next step when there are no routines", async () => {
    vi.mocked(getPresets).mockResolvedValue({
      items: [],
      total: 0,
      known_tags: [],
    })
    renderWithRouter(<RoutinesEditor />)
    expect(await screen.findByText("No routines yet")).toBeInTheDocument()
  })
})
