import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { LeafFood } from "@/lib/api/foods"
import type { Plan } from "@/lib/api/plan"
import type { Plate } from "@/lib/api/plates"
import i18n from "@/lib/i18n"
import { buildBoard, type BoardCell } from "@/lib/plan/board"
import {
  mockAc1Plates,
  mockBrownRice,
  mockPlan,
  mockPlateComponent,
  mockRemovedBatch,
  mockSession,
} from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/foods", () => ({
  getFood: vi.fn(),
  listFoodMacros: vi.fn(),
}))
vi.mock("@/lib/api/plates", () => ({
  updatePlate: vi.fn(),
  deletePlate: vi.fn(),
  updatePlateComponent: vi.fn(),
  deletePlateComponent: vi.fn(),
}))
// The fixtures are AC-1's 1–7 Nov; by default they lie in the past.
const today = vi.hoisted(() => ({ iso: "2026-11-08" }))
vi.mock("@/lib/dates", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/dates")>()),
  todayISO: () => today.iso,
}))
vi.mock("@/lib/api/feedback", () => ({
  putFeedback: vi.fn(),
  deleteFeedback: vi.fn(),
}))
vi.mock("@/lib/api/sessions", () => ({
  updateSession: vi.fn(),
  getLeftoverCells: vi.fn(),
  addLeftovers: vi.fn(),
  deleteSession: vi.fn(),
}))
vi.mock("@/lib/api/presets", () => ({ createPreset: vi.fn() }))
vi.mock("@/lib/api/availability", () => ({
  setAvailabilityOverride: vi.fn(),
  clearAvailabilityOverride: vi.fn(),
  getAvailabilityRange: vi.fn(),
}))
vi.mock("@/lib/api/plan", () => ({ restorePlan: vi.fn() }))
const toastMock = vi.hoisted(() =>
  Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn() })
)
vi.mock("@/lib/toast", () => ({ toast: toastMock, toastError: vi.fn() }))

import { setAvailabilityOverride } from "@/lib/api/availability"
import { deleteFeedback, putFeedback } from "@/lib/api/feedback"
import { getFood, listFoodMacros } from "@/lib/api/foods"
import {
  deletePlate,
  deletePlateComponent,
  updatePlate,
  updatePlateComponent,
} from "@/lib/api/plates"
import { createPreset } from "@/lib/api/presets"
import { updateSession } from "@/lib/api/sessions"

import { PlateSheet } from "./PlateSheet"

const apple: LeafFood = {
  ...mockBrownRice,
  id: 60,
  name: "Apple",
  portions: [{ food_id: 60, unit: "piece", grams: 150 }],
}

const SUNDAY = "2026-11-01|3"

function planWith(plate: Plate, patch: Partial<Plan> = {}): Plan {
  return {
    ...mockPlan,
    cells: mockPlan.cells.map((c) =>
      c.date === plate.date && c.slot_id === plate.slot_id ? { ...c, plate } : c
    ),
    ...patch,
  }
}

function cellOf(plan: Plan, key: string): BoardCell & { plate: Plate } {
  const board = buildBoard(plan, i18n)
  const cell = board.days.flatMap((d) => d.cells).find((c) => c.key === key)!
  return cell as BoardCell & { plate: Plate }
}

async function renderSheet(
  plan: Plan = mockPlan,
  key = SUNDAY,
  readOnly = false
) {
  const onPick = vi.fn()
  const onOpenCell = vi.fn()
  const onClose = vi.fn()
  const onMarkNoPlate = vi.fn()
  const view = renderWithRouter(
    <PlateSheet
      cell={cellOf(plan, key)}
      plan={plan}
      readOnly={readOnly}
      running={false}
      freezing={false}
      slotName={(id) => ["", "Breakfast", "Lunch", "Dinner"][id]}
      onPick={onPick}
      onOpenCell={onOpenCell}
      onMarkNoPlate={onMarkNoPlate}
      onClose={onClose}
    />
  )
  await screen.findAllByTestId("sheet-component")
  return { onPick, onOpenCell, onClose, onMarkNoPlate, unmount: view.unmount }
}

beforeEach(async () => {
  vi.clearAllMocks()
  today.iso = "2026-11-08"
  await i18n.changeLanguage("en")
  vi.mocked(getFood).mockResolvedValue(apple)
  vi.mocked(listFoodMacros).mockResolvedValue({
    foods: [
      {
        food_id: 42,
        macros: {
          kcal: 520,
          protein: 28,
          fat: 18,
          carbs: 60,
          fiber: 8,
          sodium: 0,
        },
      },
      {
        food_id: 60,
        macros: {
          kcal: 52,
          protein: 0.3,
          fat: 0.2,
          carbs: 14,
          fiber: 2,
          sodium: 0,
        },
      },
    ],
  })
  vi.mocked(updatePlate).mockResolvedValue(mockAc1Plates[0])
  vi.mocked(updatePlateComponent).mockResolvedValue(mockAc1Plates[0])
  vi.mocked(deletePlateComponent).mockResolvedValue(mockAc1Plates[0])
  vi.mocked(deletePlate).mockResolvedValue(mockRemovedBatch)
  vi.mocked(updateSession).mockResolvedValue(mockSession)
  vi.mocked(putFeedback).mockResolvedValue({
    plate_id: 1,
    status: "loved",
    rated_at: "2026-11-02T20:00:00Z",
  })
  vi.mocked(deleteFeedback).mockResolvedValue(undefined)
  vi.mocked(setAvailabilityOverride).mockResolvedValue({
    cell: { ...mockPlan.cells[0], mode: "none" },
    removed: mockRemovedBatch.removed,
  })
})

describe("PlateSheet", () => {
  it("shows the batch badge, the components and the batch controls", async () => {
    await renderSheet()
    expect(screen.getByText("Cook × 6")).toBeVisible()
    expect(screen.getByText("Chili")).toBeVisible()
    expect(screen.getByRole("group", { name: "Eaters" })).toHaveTextContent("2")
    expect(screen.getByTestId("batch-section")).toHaveTextContent(
      "Covers Sun dinner, Mon lunch, Mon dinner"
    )
  })

  it("edits eaters in steps of a half", async () => {
    await renderSheet()
    await userEvent.click(
      screen.getByRole("button", { name: "Increase Eaters" })
    )
    await waitFor(() =>
      expect(updatePlate).toHaveBeenCalledWith(1, { eaters: 2.5 })
    )
  })

  it("edits a recipe's portions per eater in quarters", async () => {
    await renderSheet()
    await userEvent.click(
      screen.getByRole("button", {
        name: "Increase Portions of Chili per eater",
      })
    )
    await waitFor(() =>
      expect(updatePlateComponent).toHaveBeenCalledWith(1, 9, {
        portions: 1.25,
      })
    )
  })

  // F4-AC8: kcal for one eater follow portions, not eaters.
  it("shows macros for one eater", async () => {
    const plate = mockAc1Plates[0]
    await renderSheet(planWith({ ...plate, eaters: 3 }))
    expect(await screen.findByTestId("sheet-kcal")).toHaveTextContent(
      "520 kcal"
    )
  })

  it("scales the macros with portions", async () => {
    const plate = mockAc1Plates[0]
    const larger = {
      ...plate,
      components: [{ ...plate.components[0], portions: 1.5 }],
    }
    await renderSheet(planWith(larger))
    expect(await screen.findByTestId("sheet-kcal")).toHaveTextContent(
      "780 kcal"
    )
  })

  // F4-AC8: a leaf's amount and unit per eater.
  it("edits a leaf's amount and saves it", async () => {
    const plate = {
      ...mockAc1Plates[0],
      components: [
        ...mockAc1Plates[0].components,
        mockPlateComponent({
          id: 30,
          plate_id: 1,
          food_id: 60,
          food_name: "Apple",
          kind: "leaf",
          role: null,
          portions: undefined,
          cook_session_id: undefined,
          amount: 150,
          unit: "g",
          grams: 150,
          sort_order: 1,
        }),
      ],
    }
    await renderSheet(planWith(plate))
    const amount = await screen.findByTestId("quantity-unit-amount")
    expect(amount).toHaveValue(150)
    expect(await screen.findByTestId("sheet-kcal")).toHaveTextContent(
      "598 kcal"
    )
    await userEvent.clear(amount)
    await userEvent.type(amount, "200")
    await userEvent.click(screen.getByRole("button", { name: "Update amount" }))
    await waitFor(() =>
      expect(updatePlateComponent).toHaveBeenCalledWith(1, 30, {
        amount: 200,
        unit: "g",
      })
    )
  })

  it("swaps and removes components", async () => {
    const { onPick } = await renderSheet()
    await userEvent.click(screen.getByRole("button", { name: "Swap Chili" }))
    expect(onPick).toHaveBeenCalledWith({
      kind: "swap",
      plateId: 1,
      componentId: 9,
      name: "Chili",
    })
    await userEvent.click(screen.getByRole("button", { name: "Remove Chili" }))
    await waitFor(() => expect(deletePlateComponent).toHaveBeenCalledWith(1, 9))
    await userEvent.click(screen.getByRole("button", { name: "Add food" }))
    expect(onPick).toHaveBeenLastCalledWith({ kind: "add", plateId: 1 })
  })

  // F4-AC9: the three statuses, and a second tap clears.
  it.each([
    ["Loved", "loved"],
    ["Not again", "disliked"],
    ["Didn't eat it", "not_eaten"],
  ] as const)("records %s as %s", async (label, status) => {
    await renderSheet()
    await userEvent.click(screen.getByRole("button", { name: label }))
    await waitFor(() => expect(putFeedback).toHaveBeenCalledWith(1, { status }))
  })

  it("clears the active feedback", async () => {
    const plate = {
      ...mockAc1Plates[0],
      feedback: {
        status: "disliked" as const,
        note: null,
        rated_at: "2026-11-02T20:00:00Z",
      },
    }
    await renderSheet(planWith(plate))
    const active = screen.getByRole("button", { name: "Not again" })
    expect(active).toHaveAttribute("aria-pressed", "true")
    await userEvent.click(active)
    await waitFor(() => expect(deleteFeedback).toHaveBeenCalledWith(1))
    expect(putFeedback).not.toHaveBeenCalled()
  })

  it("asks How was it? only for a plate dated today or earlier", async () => {
    today.iso = "2026-10-31"
    const { unmount } = await renderSheet()
    expect(screen.queryByTestId("feedback-loved")).toBeNull()
    unmount()

    today.iso = "2026-11-01"
    await renderSheet()
    expect(screen.getByTestId("feedback-loved")).toBeVisible()
  })

  it("offers Cook N more for a deficit", async () => {
    const short = {
      ...mockSession,
      portions: 4,
      consumed: 6,
      warnings: [{ code: "deficit" as const, plate_id: null }],
    }
    await renderSheet({ ...mockPlan, sessions: [short] })
    const warnings = screen.getByTestId("sheet-warnings")
    expect(warnings).toHaveTextContent("Chili is 2 portions short.")
    await userEvent.click(
      within(warnings).getByRole("button", { name: "Cook 2 more" })
    )
    await waitFor(() =>
      expect(updateSession).toHaveBeenCalledWith(7, {
        portions: 6,
        frozen_portions: 0,
        note: null,
      })
    )
  })

  it("formats a fractional deficit", async () => {
    const short = {
      ...mockSession,
      portions: 4,
      consumed: 4.5,
      warnings: [{ code: "deficit" as const, plate_id: null }],
    }
    await renderSheet({ ...mockPlan, sessions: [short] })
    const warnings = screen.getByTestId("sheet-warnings")
    expect(warnings).toHaveTextContent("Chili is ½ portions short.")
    expect(warnings).not.toHaveTextContent("0.5")
    expect(
      within(warnings).getByRole("button", { name: "Cook ½ more" })
    ).toBeInTheDocument()
  })

  it("Mark as No plate closes the sheet and hands the cell to the board", async () => {
    const { onClose, onMarkNoPlate } = await renderSheet()
    await userEvent.click(
      screen.getByRole("button", { name: "Mark as No plate" })
    )
    expect(onClose).toHaveBeenCalled()
    expect(onMarkNoPlate).toHaveBeenCalledWith(
      expect.objectContaining({ key: SUNDAY })
    )
  })

  it("deletes the plate and offers Undo", async () => {
    const { onClose } = await renderSheet()
    await userEvent.click(screen.getByRole("button", { name: "Delete plate" }))
    await waitFor(() => expect(deletePlate).toHaveBeenCalledWith(1))
    expect(onClose).toHaveBeenCalled()
    await waitFor(() =>
      expect(toastMock).toHaveBeenCalledWith(
        "Plate removed",
        expect.objectContaining({ duration: 6000 })
      )
    )
  })

  it("saves the plate as a routine", async () => {
    vi.mocked(createPreset).mockResolvedValue({
      id: 4,
      name: "Chili night",
      tags: [],
      plates: [],
      created_at: "",
      updated_at: "",
    })
    await renderSheet()
    await userEvent.click(
      screen.getByRole("button", { name: "Save as routine" })
    )
    const name = screen.getByTestId("sheet-routine-name")
    expect(name).toHaveValue("Chili")
    await userEvent.clear(name)
    await userEvent.type(name, "Chili night")
    await userEvent.click(
      screen.getByRole("button", { name: "Save as routine" })
    )
    await waitFor(() =>
      expect(createPreset).toHaveBeenCalledWith({
        name: "Chili night",
        plate_ids: [1],
      })
    )
    await waitFor(() =>
      expect(toastMock.success).toHaveBeenCalledWith(
        "Saved as routine “Chili night”"
      )
    )
  })

  it("allows only feedback in a past window", async () => {
    await renderSheet(mockPlan, SUNDAY, true)
    expect(
      screen.queryByRole("button", { name: "Remove Chili" })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Delete plate" })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Increase Eaters" })
    ).toBeDisabled()
    await userEvent.click(screen.getByRole("button", { name: "Loved" }))
    await waitFor(() =>
      expect(putFeedback).toHaveBeenCalledWith(1, { status: "loved" })
    )
  })

  it("saves a typed note when the sheet closes without a blur (Escape)", async () => {
    const { unmount } = await renderSheet()
    await userEvent.type(screen.getByTestId("sheet-note"), "Make it spicy")
    expect(updatePlate).not.toHaveBeenCalled()
    unmount()
    await waitFor(() =>
      expect(updatePlate).toHaveBeenCalledWith(1, { note: "Make it spicy" })
    )
  })

  it("saves a note once when it blurs and then closes", async () => {
    const { unmount } = await renderSheet()
    await userEvent.type(screen.getByTestId("sheet-note"), "Spicy")
    await userEvent.tab()
    unmount()
    await waitFor(() => expect(updatePlate).toHaveBeenCalledTimes(1))
  })
})
