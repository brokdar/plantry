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
  useUpdatePlateComponentQuantity: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useRemovePlateComponent: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useSetPlateSkipped: vi.fn(() => ({ mutateAsync: vi.fn() })),
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
      onSaveAsPreset={vi.fn()}
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

// Phase 3 — kind-aware quantity controls.
describe("SlotSheet — Phase 3 kind-aware quantity", () => {
  test("composed_component_uses_integer_stepper", async () => {
    renderSheet()
    // Composed → integer PortionStepper exposes a spinbutton with aria-valuenow.
    const sb = await screen.findByRole("spinbutton")
    expect(sb).toHaveAttribute("aria-valuenow", "1")
    expect(sb.textContent).toMatch(/×1/)
  })

  test("leaf_component_uses_quantity_unit_input", async () => {
    const leafFood: Food = {
      id: 8,
      name: "Rice",
      kind: "leaf",
      source: "manual",
      barcode: null,
      off_id: null,
      fdc_id: null,
      image_path: null,
      favorite: false,
      cook_count: 0,
      kcal_100g: 130,
      created_at: "",
      updated_at: "",
    } as unknown as Food
    const leafPlate: Plate = {
      id: 99,
      slot_id: 1,
      date: "2026-05-02",
      note: null,
      skipped: false,
      components: [
        {
          id: 200,
          plate_id: 99,
          food_id: 8,
          amount: 200,
          unit: "g",
          grams: 200,
          grams_source: "direct",
          sort_order: 0,
        },
      ],
      created_at: "",
    }
    const leafDays: PlannerDay[] = [
      { date: "2026-05-02", weekday: 5, plates: [leafPlate] },
    ]
    const leafTarget: SlotSheetTarget = {
      plateId: 99,
      date: "2026-05-02",
      weekday: 5,
      slotId: 1,
      slotNameKey: "planner.slot_dinner",
    }
    renderWithRouter(
      <SlotSheet
        target={leafTarget}
        days={leafDays}
        componentsById={new Map([[8, leafFood]])}
        rangeFrom="2026-05-02"
        rangeTo="2026-05-02"
        onOpenChange={vi.fn()}
        onAddComponent={vi.fn()}
        onSwapComponent={vi.fn()}
        onSaveAsPreset={vi.fn()}
      />
    )
    // Leaf renders the QuantityUnitInput (numeric + unit select).
    expect(await screen.findByTestId("quantity-unit-input")).toBeInTheDocument()
    const amount = (await screen.findByTestId(
      "quantity-unit-amount"
    )) as HTMLInputElement
    expect(amount.value).toBe("200")
  })
})
