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
}))
vi.mock("@/lib/queries/templates", () => ({
  useTemplates: vi.fn(),
}))

import { useFoods } from "@/lib/queries/foods"
import { useTemplates } from "@/lib/queries/templates"

function run(initial: TrayItem[], actions: TrayAction[]): TrayItem[] {
  return actions.reduce(trayReducer, initial)
}

describe("trayReducer", () => {
  it("stages a fresh food with portions=1", () => {
    const next = run([], [{ type: "stage", food: mockChickenBreast }])
    expect(next).toEqual([{ food: mockChickenBreast, portions: 1 }])
  })

  it("re-staging the same food bumps portions by 0.25", () => {
    const next = run(
      [],
      [
        { type: "stage", food: mockChickenBreast },
        { type: "stage", food: mockChickenBreast },
      ]
    )
    expect(next).toHaveLength(1)
    expect(next[0]?.portions).toBe(1.25)
  })

  it("stages multiple distinct foods preserving order", () => {
    const next = run(
      [],
      [
        { type: "stage", food: mockChickenBreast },
        { type: "stage", food: mockBrownRice },
      ]
    )
    expect(next.map((i) => i.food.id)).toEqual([1, 2])
  })

  it("portion clamps to 0.25 minimum and quantises to 0.25 grid", () => {
    let state = run([], [{ type: "stage", food: mockChickenBreast }])
    state = trayReducer(state, {
      type: "portion",
      foodId: mockChickenBreast.id,
      p: 0.1,
    })
    expect(state[0]?.portions).toBe(0.25)
    state = trayReducer(state, {
      type: "portion",
      foodId: mockChickenBreast.id,
      p: 1.37,
    })
    // 1.37 → round(*4)/4 = round(5.48)/4 = 5/4 = 1.25
    expect(state[0]?.portions).toBe(1.25)
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

  it("stages all template components, hydrating from foodsById", () => {
    const tpl: Template = {
      id: 99,
      name: "Lunch",
      created_at: "2026-04-01T00:00:00Z",
      components: [
        { id: 1, template_id: 99, food_id: 1, portions: 2, sort_order: 0 },
        { id: 2, template_id: 99, food_id: 2, portions: 1.5, sort_order: 1 },
      ],
    }
    const foodsById = new Map<number, Food>([
      [1, mockChickenBreast],
      [2, mockBrownRice],
    ])
    const next = run([], [{ type: "stageTemplate", template: tpl, foodsById }])
    expect(next).toEqual([
      { food: mockChickenBreast, portions: 2 },
      { food: mockBrownRice, portions: 1.5 },
    ])
  })

  it("stageTemplate adds onto existing tray entries by summing portions", () => {
    const tpl: Template = {
      id: 99,
      name: "Lunch",
      created_at: "2026-04-01T00:00:00Z",
      components: [
        { id: 1, template_id: 99, food_id: 1, portions: 0.5, sort_order: 0 },
      ],
    }
    const foodsById = new Map<number, Food>([[1, mockChickenBreast]])
    const next = run(
      [{ food: mockChickenBreast, portions: 1 }],
      [{ type: "stageTemplate", template: tpl, foodsById }]
    )
    expect(next).toEqual([{ food: mockChickenBreast, portions: 1.5 }])
  })

  it("stageTemplate skips entries whose foodId isn't in foodsById", () => {
    const tpl: Template = {
      id: 99,
      name: "Lunch",
      created_at: "2026-04-01T00:00:00Z",
      components: [
        { id: 1, template_id: 99, food_id: 1, portions: 1, sort_order: 0 },
        { id: 2, template_id: 99, food_id: 999, portions: 1, sort_order: 1 },
      ],
    }
    const foodsById = new Map<number, Food>([[1, mockChickenBreast]])
    const next = run([], [{ type: "stageTemplate", template: tpl, foodsById }])
    expect(next).toEqual([{ food: mockChickenBreast, portions: 1 }])
  })

  it("stage handles a composed food the same way", () => {
    const next = run([], [{ type: "stage", food: mockChickenCurry }])
    expect(next).toEqual([{ food: mockChickenCurry, portions: 1 }])
  })

  it("keepOnly drops everything not in the foodIds set", () => {
    const state: TrayItem[] = [
      { food: mockChickenBreast, portions: 1 },
      { food: mockBrownRice, portions: 2 },
      { food: mockChickenCurry, portions: 1.5 },
    ]
    const next = trayReducer(state, {
      type: "keepOnly",
      foodIds: new Set([mockBrownRice.id]),
    })
    expect(next).toEqual([{ food: mockBrownRice, portions: 2 }])
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
    expect(onCommit).toHaveBeenCalledWith(
      [{ food_id: mockChickenBreast.id, portions: 1 }],
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
          components: [
            {
              id: 1,
              template_id: 7,
              food_id: mockChickenBreast.id,
              portions: 1,
              sort_order: 0,
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
})
