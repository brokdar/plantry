import { screen } from "@testing-library/react"
import { describe, expect, test, vi } from "vitest"

import "@/lib/i18n"
import { renderWithRouter } from "@/test/render"
import type { Food } from "@/lib/api/foods"
import type { Plate } from "@/lib/api/plates"

import type { PlannerDay } from "./PlannerGrid"
import { SlotSheet, type SlotSheetTarget } from "./SlotSheet"

// Mock the hooks the sheet depends on so it renders without a live backend.
vi.mock("@/lib/queries/plates", () => ({
  useUpdatePlate: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useUpdatePlateComponentPortions: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useRemovePlateComponent: vi.fn(() => ({ mutateAsync: vi.fn() })),
  // The header summary + per-component contribution both read from these.
  usePlateMacros: vi.fn(() => ({
    data: {
      plates: [
        {
          plate_id: 42,
          date: "2026-05-02",
          skipped: false,
          macros: {
            kcal: 620,
            protein: 30,
            fat: 15,
            carbs: 60,
            fiber: 4,
            sodium: 200,
          },
        },
      ],
    },
  })),
}))

vi.mock("@/lib/queries/foods", () => ({
  useFoodMacros: vi.fn(() => ({
    data: {
      foods: [
        {
          food_id: 7,
          // Per-portion macros for the composed hero food: 620 kcal × 1 portion
          // gives the row contribution shown by the test below.
          macros: {
            kcal: 620,
            protein: 30,
            fat: 15,
            carbs: 60,
            fiber: 4,
            sodium: 200,
          },
        },
      ],
    },
  })),
}))

vi.mock("@/lib/queries/feedback", () => ({
  useRecordFeedback: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useClearFeedback: vi.fn(() => ({ mutateAsync: vi.fn() })),
}))

const heroFood: Food = {
  id: 7,
  name: "Bolognese",
  kind: "composed",
  role: "main",
  image_path: null,
  favorite: false,
  cook_count: 0,
  last_cooked_at: null,
  created_at: "2026-05-01T00:00:00Z",
  updated_at: "2026-05-01T00:00:00Z",
  children: [],
  instructions: [],
  tags: [],
} as unknown as Food

const plate: Plate = {
  id: 42,
  slot_id: 1,
  date: "2026-05-02",
  note: null,
  skipped: false,
  components: [
    { id: 100, plate_id: 42, food_id: 7, portions: 1, sort_order: 0 },
  ],
  created_at: "2026-05-02T10:00:00Z",
}

const days: PlannerDay[] = [{ date: "2026-05-02", weekday: 5, plates: [plate] }]

const target: SlotSheetTarget = {
  plateId: 42,
  date: "2026-05-02",
  weekday: 5,
  slotId: 1,
  slotNameKey: "planner.slot_dinner",
}

function renderSheet() {
  return renderWithRouter(
    <SlotSheet
      target={target}
      days={days}
      componentsById={new Map([[7, heroFood]])}
      rangeFrom="2026-05-02"
      rangeTo="2026-05-02"
      onOpenChange={vi.fn()}
      onAddComponent={vi.fn()}
      onSwapComponent={vi.fn()}
      onSaveAsTemplate={vi.fn()}
      onToggleSkip={vi.fn()}
      onDeletePlate={vi.fn()}
    />
  )
}

describe("SlotSheet — Phase 2 macros", () => {
  test("renders the macro summary in the header", async () => {
    renderSheet()
    const kcal = await screen.findByTestId("slot-sheet-kcal")
    expect(kcal.textContent).toMatch(/620/)
  })

  test("renders per-component kcal contribution next to the stepper", async () => {
    renderSheet()
    const row = await screen.findByTestId("slot-sheet-row-kcal-100")
    // Composed food, 1 portion × 620 kcal/portion = 620 kcal contribution.
    expect(row.textContent).toMatch(/620/)
  })
})
