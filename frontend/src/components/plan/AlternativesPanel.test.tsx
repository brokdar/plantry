import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { Alternative } from "@/lib/api/meal"
import i18n from "@/lib/i18n"
import { edited, mockPlan, mockPlate } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/meal", () => ({
  getAlternatives: vi.fn(),
  replaceComponent: vi.fn(),
  revertPlan: vi.fn(),
}))
vi.mock("@/lib/api/plates", () => ({
  addPlateComponent: vi.fn(),
  deletePlateComponent: vi.fn(),
}))
const toastMock = vi.hoisted(() =>
  Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn() })
)
vi.mock("@/lib/toast", () => ({ toast: toastMock, toastError: vi.fn() }))

import { getAlternatives, replaceComponent } from "@/lib/api/meal"

import { AlternativesPanel, chipConstraints } from "./AlternativesPanel"

const rows: Alternative[] = [
  {
    food_id: 11,
    name: "Dal",
    source: "fresh",
    total_minutes: 25,
    kcal: 540,
    protein_g: 22,
    day_kcal: 1900,
    day_pct: 86,
    cook_occasions_day: 1,
    new_issues: [
      { code: "no_such_code", rule: "S9", severity: "soft", params: {} },
    ] as unknown as Alternative["new_issues"],
    ops: [{ op: "swap", plate_component_id: 3, food_id: 11 }],
  },
  {
    food_id: 12,
    name: "Chili",
    source: "session",
    session_id: 7,
    kcal: 600,
    protein_g: 30,
    day_kcal: 2100,
    cook_occasions_day: 0,
    ops: [{ op: "swap", plate_component_id: 3, food_id: 12 }],
  },
]

function renderPanel(onDone = vi.fn()) {
  renderWithRouter(
    <AlternativesPanel
      plan={mockPlan}
      plateId={3}
      date="2026-11-02"
      target={{ what: "meal", title: "Something else" }}
      describe={() => undefined}
      onBack={vi.fn()}
      onDone={onDone}
    />
  )
  return onDone
}

beforeEach(async () => {
  vi.clearAllMocks()
  await i18n.changeLanguage("en")
  vi.mocked(getAlternatives).mockResolvedValue({ alternatives: rows })
})

describe("chipConstraints", () => {
  it("maps each chip to its constraint and nothing to undefined", () => {
    expect(chipConstraints(new Set(), 700)).toBeUndefined()
    expect(
      chipConstraints(
        new Set(["quicker", "vegetarian", "protein", "lighter"]),
        700
      )
    ).toEqual({
      max_minutes: 30,
      diet: "vegetarian",
      min_protein_g: 25,
      max_kcal: 700,
    })
  })

  it("Lighter needs a kcal share", () => {
    expect(chipConstraints(new Set(["lighter"]), undefined)).toBeUndefined()
  })
})

describe("AlternativesPanel", () => {
  it("shows each row's minutes, kcal, effect on the day and badge", async () => {
    renderPanel()
    const list = await screen.findAllByTestId("alt-row")
    expect(list).toHaveLength(2)
    expect(within(list[0]).getByText("Dal")).toBeInTheDocument()
    expect(within(list[0]).getByText("Adds a cooking")).toBeInTheDocument()
    expect(list[0]).toHaveTextContent("25 min")
    expect(list[0]).toHaveTextContent("540")
    expect(list[0]).toHaveTextContent("Day 86 %")
    expect(within(list[1]).getByText("Eats a batch")).toBeInTheDocument()
    expect(list[1]).toHaveTextContent("Day 2,100 kcal")
  })

  it("asks the backend for the plate and refetches with a chip's constraint", async () => {
    renderPanel()
    await screen.findAllByTestId("alt-row")
    expect(getAlternatives).toHaveBeenLastCalledWith(
      mockPlan.window.start,
      expect.objectContaining({ plate_id: 3, what: "meal" })
    )
    const quicker = screen.getByRole("button", { name: "Quicker" })
    await userEvent.click(quicker)
    expect(quicker).toHaveAttribute("aria-pressed", "true")
    await waitFor(() =>
      expect(getAlternatives).toHaveBeenLastCalledWith(
        mockPlan.window.start,
        expect.objectContaining({ constraints: { max_minutes: 30 } })
      )
    )
    await userEvent.click(screen.getByRole("button", { name: "Vegetarian" }))
    await waitFor(() =>
      expect(getAlternatives).toHaveBeenLastCalledWith(
        mockPlan.window.start,
        expect.objectContaining({
          constraints: { max_minutes: 30, diet: "vegetarian" },
        })
      )
    )
  })

  it("choosing a row applies it, offers Undo and closes", async () => {
    vi.mocked(replaceComponent).mockResolvedValue(
      edited({ plates: [mockPlate()], session_ids: [] })
    )
    const onDone = renderPanel()
    const list = await screen.findAllByTestId("alt-row")
    await userEvent.click(list[0])
    await waitFor(() => expect(onDone).toHaveBeenCalled())
    expect(replaceComponent).toHaveBeenCalledWith(3, {
      plate_component_id: 3,
      food_id: 11,
    })
    expect(toastMock).toHaveBeenCalledWith("Meal changed", expect.anything())
  })

  it("disables the chips and rows while a row is being applied", async () => {
    vi.mocked(replaceComponent).mockReturnValue(new Promise(() => {}))
    renderPanel()
    const list = await screen.findAllByTestId("alt-row")
    await userEvent.click(list[0])
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Quicker" })).toBeDisabled()
    )
    expect(screen.getByRole("button", { name: "Vegetarian" })).toBeDisabled()
    for (const row of screen.getAllByTestId("alt-row"))
      expect(row).toBeDisabled()
  })

  it("says so when nothing fits", async () => {
    vi.mocked(getAlternatives).mockResolvedValue({ alternatives: [] })
    renderPanel()
    expect(
      await screen.findByText("Nothing fits. Try fewer filters.")
    ).toBeInTheDocument()
  })

  it("shows the load error", async () => {
    vi.mocked(getAlternatives).mockRejectedValue(new Error("down"))
    renderPanel()
    expect(
      await screen.findByText("Couldn't load the alternatives.")
    ).toBeInTheDocument()
  })
})
