import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithRouter } from "@/test/render"
import {
  ComponentTraySheet,
  trayReducer,
  type TrayAction,
  type TrayCommitResult,
  type TrayItem,
  type TraySlotContext,
} from "./ComponentTraySheet"
import {
  mockBrownRice,
  mockChickenBreast,
  mockChickenCurry,
} from "@/test/fixtures"
import type { Food } from "@/lib/api/foods"
import type { Template } from "@/lib/api/templates"

vi.mock("@/lib/queries/foods", () => ({
  useFoods: vi.fn(),
  useFoodMacros: vi.fn(() => ({ data: { foods: [] } })),
}))
vi.mock("@/lib/queries/templates", () => ({
  useTemplates: vi.fn(),
}))

import { useFoodMacros, useFoods } from "@/lib/queries/foods"
import { useTemplates } from "@/lib/queries/templates"

function run(initial: TrayItem[], actions: TrayAction[]): TrayItem[] {
  return actions.reduce(trayReducer, initial)
}

describe("trayReducer", () => {
  it("stage_composed_initialises_portions_1", () => {
    const next = run([], [{ type: "stage", food: mockChickenCurry }])
    expect(next).toEqual([
      {
        food: mockChickenCurry,
        quantity: { kind: "composed", portions: 1 },
      },
    ])
  })

  it("stage_leaf_initialises_with_default_unit_and_amount", () => {
    // Brown rice has no portion table → default 100 g.
    const next = run([], [{ type: "stage", food: mockBrownRice }])
    expect(next).toEqual([
      {
        food: mockBrownRice,
        quantity: { kind: "leaf", amount: 100, unit: "g" },
      },
    ])
  })

  it("stage_existing_composed_increments_portions_by_1", () => {
    const next = run(
      [],
      [
        { type: "stage", food: mockChickenCurry },
        { type: "stage", food: mockChickenCurry },
      ]
    )
    expect(next).toHaveLength(1)
    expect(next[0]?.quantity).toEqual({ kind: "composed", portions: 2 })
  })

  it("stage_existing_leaf_increments_amount_by_50_when_grams", () => {
    const next = run(
      [],
      [
        { type: "stage", food: mockBrownRice },
        { type: "stage", food: mockBrownRice },
      ]
    )
    expect(next).toHaveLength(1)
    expect(next[0]?.quantity).toEqual({ kind: "leaf", amount: 150, unit: "g" })
  })

  it("stage_existing_leaf_increments_amount_by_1_unit_when_count", () => {
    const apple: Food = {
      ...mockChickenBreast,
      id: 50,
      name: "Apple",
      portions: [{ food_id: 50, unit: "apple", grams: 180 }],
    }
    const next = run(
      [],
      [
        { type: "stage", food: apple },
        { type: "stage", food: apple },
      ]
    )
    expect(next).toHaveLength(1)
    expect(next[0]?.quantity).toEqual({
      kind: "leaf",
      amount: 2,
      unit: "apple",
    })
  })

  it("commit_payload_for_composed_uses_portions_only", () => {
    const next = run([], [{ type: "stage", food: mockChickenCurry }])
    expect(next).toEqual([
      {
        food: mockChickenCurry,
        quantity: { kind: "composed", portions: 1 },
      },
    ])
  })

  it("commit_payload_for_leaf_uses_amount_unit", () => {
    const next = run([], [{ type: "stage", food: mockBrownRice }])
    expect(next).toEqual([
      {
        food: mockBrownRice,
        quantity: { kind: "leaf", amount: 100, unit: "g" },
      },
    ])
  })

  it("quantity action replaces the staged quantity wholesale", () => {
    let state = run([], [{ type: "stage", food: mockBrownRice }])
    state = trayReducer(state, {
      type: "quantity",
      foodId: mockBrownRice.id,
      q: { kind: "leaf", amount: 250, unit: "g" },
    })
    expect(state[0]?.quantity).toEqual({
      kind: "leaf",
      amount: 250,
      unit: "g",
    })
  })

  it("removes a staged food", () => {
    let state = run(
      [],
      [
        { type: "stage", food: mockChickenBreast },
        { type: "stage", food: mockBrownRice },
      ]
    )
    state = trayReducer(state, { type: "remove", foodId: mockChickenBreast.id })
    expect(state.map((i) => i.food.id)).toEqual([2])
  })

  it("stageTemplate hydrates per-food kind-aware quantities", () => {
    const tpl: Template = {
      id: 99,
      name: "Lunch",
      created_at: "2026-04-01T00:00:00Z",
      scope: "slot",
      components: [
        {
          id: 1,
          template_id: 99,
          food_id: 3, // composed (chicken curry)
          portions: 2,
          sort_order: 0,
          day_offset: 0,
        },
        {
          id: 2,
          template_id: 99,
          food_id: 2, // leaf (brown rice)
          portions: 1.5,
          sort_order: 1,
          day_offset: 0,
        },
      ],
    }
    const foodsById = new Map<number, Food>([
      [3, mockChickenCurry],
      [2, mockBrownRice],
    ])
    const next = run([], [{ type: "stageTemplate", template: tpl, foodsById }])
    expect(next).toEqual([
      {
        food: mockChickenCurry,
        quantity: { kind: "composed", portions: 2 },
      },
      // Leaf templates legacy-portion 1.5 → 150 g (1.5 × 100 g).
      {
        food: mockBrownRice,
        quantity: { kind: "leaf", amount: 150, unit: "g" },
      },
    ])
  })

  it("keepOnly drops everything not in the foodIds set", () => {
    const state: TrayItem[] = [
      {
        food: mockChickenBreast,
        quantity: { kind: "leaf", amount: 100, unit: "g" },
      },
      {
        food: mockBrownRice,
        quantity: { kind: "leaf", amount: 200, unit: "g" },
      },
      {
        food: mockChickenCurry,
        quantity: { kind: "composed", portions: 1 },
      },
    ]
    const next = trayReducer(state, {
      type: "keepOnly",
      foodIds: new Set([mockBrownRice.id]),
    })
    expect(next).toEqual([
      {
        food: mockBrownRice,
        quantity: { kind: "leaf", amount: 200, unit: "g" },
      },
    ])
  })
})

// ── Integration: sheet UI ───────────────────────────────────────────

const ctx: TraySlotContext = {
  slotId: 1,
  slotNameKey: "planner.slot_lunch",
  date: "2026-05-04",
  weekday: 0,
}

function setupQueries(opts?: {
  foods?: Food[]
  templates?: Template[]
  foodsError?: boolean
}) {
  ;(useFoods as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
    data: { items: opts?.foods ?? [mockChickenBreast, mockBrownRice] },
    isLoading: false,
    isError: !!opts?.foodsError,
  })
  ;(useTemplates as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
    data: opts?.templates ?? [],
    isLoading: false,
    isError: false,
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear?.()
})

describe("ComponentTraySheet integration", () => {
  it("stages a food then commits with the right items", async () => {
    setupQueries()
    const onCommit = vi.fn(
      async (): Promise<TrayCommitResult> => ({ failedFoodIds: [] })
    )
    const onOpenChange = vi.fn()
    renderWithRouter(
      <ComponentTraySheet
        open
        context={ctx}
        recentFoods={[]}
        onOpenChange={onOpenChange}
        onCommit={onCommit}
      />
    )

    const user = userEvent.setup()
    const result = await screen.findByTestId(
      `tray-result-${mockChickenBreast.id}`
    )
    await user.click(result)

    expect(
      await screen.findByTestId(`tray-staged-${mockChickenBreast.id}`)
    ).toBeInTheDocument()

    await user.click(screen.getByTestId("tray-commit"))

    await waitFor(() => expect(onCommit).toHaveBeenCalledTimes(1))
    // Chicken breast is a leaf food → commit shape carries amount + unit,
    // never portions. The default for a leaf without a portion table is
    // `{ amount: 100, unit: "g" }`.
    expect(onCommit).toHaveBeenCalledWith(
      [{ food_id: mockChickenBreast.id, amount: 100, unit: "g" }],
      ctx
    )
    // Full success closes the sheet.
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
  })

  it("commit button is disabled until something is staged", async () => {
    setupQueries()
    renderWithRouter(
      <ComponentTraySheet
        open
        context={ctx}
        recentFoods={[]}
        onOpenChange={vi.fn()}
        onCommit={vi.fn()}
      />
    )
    const commit = await screen.findByTestId("tray-commit")
    expect(commit).toBeDisabled()
  })

  it("keeps failed items staged on partial commit failure", async () => {
    setupQueries()
    const onCommit = vi.fn(
      async (): Promise<TrayCommitResult> => ({
        failedFoodIds: [mockBrownRice.id],
      })
    )
    const onOpenChange = vi.fn()
    renderWithRouter(
      <ComponentTraySheet
        open
        context={ctx}
        recentFoods={[]}
        onOpenChange={onOpenChange}
        onCommit={onCommit}
      />
    )

    const user = userEvent.setup()
    await user.click(
      await screen.findByTestId(`tray-result-${mockChickenBreast.id}`)
    )
    await user.click(
      await screen.findByTestId(`tray-result-${mockBrownRice.id}`)
    )
    await user.click(await screen.findByTestId("tray-commit"))

    await waitFor(() => expect(onCommit).toHaveBeenCalledTimes(1))
    // Sheet stays open; the failed item remains staged, the succeeded one
    // is gone.
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
    await waitFor(() =>
      expect(
        screen.queryByTestId(`tray-staged-${mockChickenBreast.id}`)
      ).not.toBeInTheDocument()
    )
    expect(
      screen.getByTestId(`tray-staged-${mockBrownRice.id}`)
    ).toBeInTheDocument()
  })

  it("renders an error alert when foods fail to load", async () => {
    setupQueries({ foodsError: true })
    renderWithRouter(
      <ComponentTraySheet
        open
        context={ctx}
        recentFoods={[]}
        onOpenChange={vi.fn()}
        onCommit={vi.fn()}
      />
    )
    const err = await screen.findByTestId("tray-error")
    expect(err).toBeInTheDocument()
  })

  it("switches to the templates tab and lists templates", async () => {
    setupQueries({
      templates: [
        {
          id: 7,
          name: "Weekday Lunch",
          created_at: "2026-04-01T00:00:00Z",
          scope: "slot",
          components: [
            {
              id: 1,
              template_id: 7,
              food_id: mockChickenBreast.id,
              portions: 1,
              sort_order: 0,
              day_offset: 0,
            },
          ],
        },
      ],
    })
    renderWithRouter(
      <ComponentTraySheet
        open
        context={ctx}
        recentFoods={[]}
        onOpenChange={vi.fn()}
        onCommit={vi.fn()}
      />
    )
    const user = userEvent.setup()
    await user.click(await screen.findByTestId("tray-tab-templates"))
    expect(await screen.findByTestId("tray-template-7")).toBeInTheDocument()
  })

  it("running total updates when staging a food", async () => {
    setupQueries()
    // Stub useFoodMacros so the running total has macros to multiply by.
    ;(useFoodMacros as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
      data: {
        foods: [
          {
            food_id: mockChickenBreast.id,
            macros: {
              kcal: 165,
              protein: 31,
              fat: 3.6,
              carbs: 0,
              fiber: 0,
              sodium: 70,
            },
          },
        ],
      },
    })

    renderWithRouter(
      <ComponentTraySheet
        open
        context={ctx}
        recentFoods={[]}
        onOpenChange={vi.fn()}
        onCommit={vi.fn()}
      />
    )

    const user = userEvent.setup()
    const result = await screen.findByTestId(
      `tray-result-${mockChickenBreast.id}`
    )
    await user.click(result)

    // Default portions = 1; running total = round(165 * 1) = 165.
    const total = await screen.findByTestId("tray-running-kcal")
    await waitFor(() => expect(total.textContent).toMatch(/165/))
  })
})
