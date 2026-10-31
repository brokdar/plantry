import { beforeEach, describe, expect, it } from "vitest"

import type { Plan } from "@/lib/api/plan"
import i18n from "@/lib/i18n"
import {
  mockPlan,
  mockPlate,
  mockPlateComponent,
  mockSession,
  mockSessionWithWarnings,
} from "@/test/fixtures"

import { buildBoard, copySnapshot, type BoardCell } from "./board"
import { batchBadgeText } from "./format"

function cellAt(
  board: ReturnType<typeof buildBoard>,
  date: string,
  slotId: number
): BoardCell {
  const cell = board.days
    .find((d) => d.date === date)
    ?.cells.find((c) => c.slotId === slotId)
  if (!cell) throw new Error(`no cell ${date}|${slotId}`)
  return cell
}

/** mockPlan with every cell of slot 1 set to `mode` (and preset Oats). */
function withBreakfast(
  mode: "routine" | "plan",
  override?: (date: string) => Partial<Plan["cells"][number]>
): Plan {
  return {
    ...mockPlan,
    cells: mockPlan.cells.map((c) =>
      c.slot_id === 1
        ? {
            ...c,
            mode,
            preset_id: mode === "routine" ? 5 : null,
            preset_name: mode === "routine" ? "Oats" : null,
            ...override?.(c.date),
          }
        : c
    ),
  }
}

describe("buildBoard", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("en")
  })

  it("F3-AC1: gives the AC-1 batch its cook and leftover badges in one colour", () => {
    const board = buildBoard(mockPlan, i18n)

    const cook = cellAt(board, "2026-11-01", 3)
    const lunch = cellAt(board, "2026-11-02", 2)
    const dinner = cellAt(board, "2026-11-02", 3)

    expect(cook.badge).toMatchObject({ variant: "cook", portions: 6 })
    expect(batchBadgeText(cook.badge!, i18n)).toBe("Cook × 6")
    expect(batchBadgeText(lunch.badge!, i18n)).toBe("2/3")
    expect(batchBadgeText(dinner.badge!, i18n)).toBe("3/3")

    expect(board.cookStrip).toHaveLength(1)
    const colour = board.cookStrip[0].colour
    expect([cook, lunch, dinner].map((c) => c.badge?.colour)).toEqual([
      colour,
      colour,
      colour,
    ])

    expect(cook.subline).toBe("cook")
    expect(cook.cookOf).toEqual([7])
    expect(lunch.subline).toBe("leftover")
    expect(lunch.cookOf).toEqual([])
    expect(lunch.sessionIds).toEqual([7])
  })

  it("lays out 7 days from the window start, each with its slots and meter", () => {
    const board = buildBoard(mockPlan, i18n)
    expect(board.days.map((d) => d.date)).toEqual([
      "2026-10-31",
      "2026-11-01",
      "2026-11-02",
      "2026-11-03",
      "2026-11-04",
      "2026-11-05",
      "2026-11-06",
    ])
    expect(board.days[0].weekdayShort).toBe("Sat")
    expect(board.days[0].dayNumber).toBe("31")
    expect(board.days[0].isToday).toBe(true)
    expect(board.days[2].nutrition?.macros.kcal).toBe(1040)
    expect(board.rows.map((r) => r.name)).toEqual([
      "Breakfast",
      "Lunch",
      "Dinner",
    ])
    expect(board.rows[2].cells).toHaveLength(7)
    expect(board.empty).toBe(false)
    expect(board.readOnly).toBe(false)
  })

  it("names a cell in full for screen readers", () => {
    const board = buildBoard(mockPlan, i18n)
    expect(cellAt(board, "2026-11-02", 3).ariaLabel).toBe(
      "Monday dinner, Chili, leftover 3 of 3"
    )
    expect(cellAt(board, "2026-11-01", 3).ariaLabel).toBe(
      "Sunday dinner, Chili, cook here, 6 portions"
    )
    expect(cellAt(board, "2026-11-03", 1).ariaLabel).toBe(
      "Tuesday breakfast, nothing planned"
    )
  })

  it("says one portion in the singular", async () => {
    const one = { ...mockPlan, sessions: [{ ...mockSession, portions: 1 }] }
    expect(cellAt(buildBoard(one, i18n), "2026-11-01", 3).ariaLabel).toBe(
      "Sunday dinner, Chili, cook here, 1 portion"
    )
    await i18n.changeLanguage("de")
    try {
      expect(cellAt(buildBoard(one, i18n), "2026-11-01", 3).ariaLabel).toBe(
        "Sonntag Abendessen, Chili, hier kochen, 1 Portion"
      )
    } finally {
      await i18n.changeLanguage("en")
    }
  })

  it("F3-AC2: collapses an all-routine breakfast row", () => {
    const board = buildBoard(withBreakfast("routine"), i18n)
    expect(board.rows[0].collapsed).toBe("Breakfast: Oats every day")
    expect(board.rows[1].collapsed).toBeNull()
  })

  it("F3-AC2: does not collapse a routine row with one plan cell", () => {
    const board = buildBoard(
      withBreakfast("routine", (date) =>
        date === "2026-11-03"
          ? { mode: "plan", preset_id: null, preset_name: null }
          : {}
      ),
      i18n
    )
    expect(board.rows[0].collapsed).toBeNull()
    expect(cellAt(board, "2026-11-04", 1).subline).toBe("routine")
  })

  it("labels no-plate and routine cells", () => {
    const plan = withBreakfast("routine", (date) =>
      date === "2026-11-03" ? { mode: "none", note: "Hotel breakfast" } : {}
    )
    const board = buildBoard(plan, i18n)
    expect(cellAt(board, "2026-11-03", 1).ariaLabel).toBe(
      "Tuesday breakfast, No plate, Hotel breakfast"
    )
    expect(cellAt(board, "2026-11-04", 1).ariaLabel).toBe(
      "Wednesday breakfast, Routine, Oats"
    )
  })

  it("names the main food and the sides", () => {
    const plate = mockPlate({
      id: 40,
      date: "2026-11-05",
      slot_id: 3,
      components: [
        mockPlateComponent({
          id: 41,
          food_name: "Rice",
          role: "side_starch",
          cook_session_id: undefined,
          sort_order: 1,
        }),
        mockPlateComponent({
          id: 42,
          food_name: "Salad",
          role: "side_veg",
          cook_session_id: undefined,
          sort_order: 2,
        }),
        mockPlateComponent({
          id: 43,
          food_name: "Curry",
          role: "main",
          cook_session_id: undefined,
          sort_order: 0,
        }),
      ],
    })
    const plan: Plan = {
      ...mockPlan,
      cells: mockPlan.cells.map((c) =>
        c.date === "2026-11-05" && c.slot_id === 3 ? { ...c, plate } : c
      ),
    }
    const cell = cellAt(buildBoard(plan, i18n), "2026-11-05", 3)
    expect(cell.mainName).toBe("Curry")
    expect(cell.sideText).toBe("with Rice and Salad")
    expect(cell.badge).toBeNull()
  })

  it("puts warnings on their plates and session-wide ones on the cook cell", () => {
    const plan: Plan = {
      ...mockPlan,
      sessions: [{ ...mockSessionWithWarnings, id: 7 }],
    }
    const board = buildBoard(plan, i18n)
    expect(cellAt(board, "2026-11-01", 3).warnings).toEqual(["deficit"])
    expect(cellAt(board, "2026-11-02", 2).warnings).toEqual([
      "leftover_before_cook",
    ])
    expect(board.warningCells).toEqual(["2026-11-01|3", "2026-11-02|2"])
    expect(cellAt(board, "2026-11-02", 2).ariaLabel).toMatch(/needs attention$/)
  })

  it("leaves sessions cooked before the window out of the cook strip", () => {
    const plan: Plan = {
      ...mockPlan,
      window: { ...mockPlan.window, start: "2026-11-02", end: "2026-11-08" },
    }
    expect(buildBoard(plan, i18n).cookStrip).toHaveLength(0)
  })

  it("marks a past window read-only and an empty one empty", () => {
    const plan: Plan = {
      ...mockPlan,
      window: { ...mockPlan.window, today: "2026-11-20", is_current: false },
      cells: mockPlan.cells.map((c) => ({ ...c, plate: null })),
    }
    const board = buildBoard(plan, i18n)
    expect(board.readOnly).toBe(true)
    expect(board.empty).toBe(true)
  })
})

describe("copySnapshot", () => {
  it("builds a one-plate snapshot at the target with no session links", () => {
    const plate = mockPlate({
      id: 9,
      eaters: 2,
      note: "Spicy",
      components: [
        mockPlateComponent({
          id: 1,
          food_id: 42,
          portions: 1.5,
          cook_session_id: 7,
        }),
        mockPlateComponent({
          id: 2,
          food_id: 5,
          kind: "leaf",
          role: null,
          portions: undefined,
          cook_session_id: undefined,
          amount: 1,
          unit: "piece",
          grams: 150,
          sort_order: 1,
        }),
      ],
    })
    expect(copySnapshot(plate, "2026-11-03", 3)).toEqual({
      plates: [
        {
          date: "2026-11-03",
          slot_id: 3,
          eaters: 2,
          note: "Spicy",
          components: [
            { food_id: 42, portions: 1.5, sort_order: 0 },
            { food_id: 5, amount: 1, unit: "piece", grams: 150, sort_order: 1 },
          ],
        },
      ],
      sessions: [],
    })
  })
})
