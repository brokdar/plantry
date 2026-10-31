import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, test, vi } from "vitest"

import "@/lib/i18n"
import { renderWithRouter } from "@/test/render"
import type { Food } from "@/lib/api/foods"
import type { Plate } from "@/lib/api/plates"

import type { PlannerDay } from "./PlannerGrid"
import { SlotSheet, type SlotSheetTarget } from "./SlotSheet"
import { mockPlate, mockPlateComponent } from "@/test/fixtures"

const mocks = vi.hoisted(() => ({ markNoPlate: vi.fn() }))

// Mock the hooks the sheet depends on so it renders without a live backend.
vi.mock("@/lib/queries/plates", () => ({
  usePlannerUpdatePlate: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useUpdatePlateComponentQuantity: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useRemovePlateComponent: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useNoPlateActions: vi.fn(() => ({
    markNoPlate: mocks.markNoPlate,
    planMeal: vi.fn(),
    editNote: vi.fn(),
  })),
  // The header summary + per-component contribution both read from these.
  usePlateMacros: vi.fn(() => ({
    data: {
      plates: [
        {
          plate_id: 42,
          date: "2026-05-02",
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
  created_at: "2026-05-01T00:00:00Z",
  updated_at: "2026-05-01T00:00:00Z",
  children: [],
  instructions: [],
  tags: [],
} as unknown as Food

const plate: Plate = mockPlate({
  id: 42,
  slot_id: 1,
  date: "2026-05-02",
  components: [mockPlateComponent({ id: 100, plate_id: 42, food_id: 7 })],
})

const days: PlannerDay[] = [{ date: "2026-05-02", weekday: 5, plates: [plate] }]

const target: SlotSheetTarget = {
  plateId: 42,
  date: "2026-05-02",
  weekday: 5,
  slotId: 1,
  slotNameKey: "planner.slot_dinner",
}

function renderSheet(food: Food = heroFood) {
  return renderWithRouter(
    <SlotSheet
      target={target}
      days={days}
      componentsById={new Map([[7, food]])}
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

describe("SlotSheet — hero title contrast", () => {
  // The hero title is the large heading-font name over the hero image.
  async function heroTitle() {
    const matches = await screen.findAllByText("Bolognese")
    return matches.find((el) => el.classList.contains("font-heading"))!
  }

  test("uses ink text over a placeholder", async () => {
    renderSheet()
    const title = await heroTitle()
    expect(title).toHaveClass("text-ink")
    expect(title).not.toHaveClass("text-white")
  })

  test("uses white text over a photo", async () => {
    renderSheet({ ...heroFood, image_path: "bolognese.jpg" } as Food)
    const title = await heroTitle()
    expect(title).toHaveClass("text-white")
    expect(title).not.toHaveClass("text-ink")
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
      kcal_100g: 130,
      created_at: "",
      updated_at: "",
    } as unknown as Food
    const leafPlate: Plate = mockPlate({
      id: 99,
      slot_id: 1,
      date: "2026-05-02",
      components: [
        {
          id: 200,
          plate_id: 99,
          food_id: 8,
          food_name: "Rice",
          kind: "leaf",
          amount: 200,
          unit: "g",
          grams: 200,
          grams_source: "direct",
          sort_order: 0,
        },
      ],
    })
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

describe("SlotSheet — Mark as No plate", () => {
  test("marks the cell No plate through the shared action and closes", async () => {
    const onOpenChange = vi.fn()
    renderWithRouter(
      <SlotSheet
        target={target}
        days={days}
        componentsById={new Map([[7, heroFood]])}
        rangeFrom="2026-05-02"
        rangeTo="2026-05-02"
        onOpenChange={onOpenChange}
        onAddComponent={vi.fn()}
        onSwapComponent={vi.fn()}
        onSaveAsPreset={vi.fn()}
      />
    )

    await userEvent.click(
      await screen.findByRole("button", { name: "Mark as No plate" })
    )

    expect(mocks.markNoPlate).toHaveBeenCalledWith("2026-05-02", 1)
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })
})
