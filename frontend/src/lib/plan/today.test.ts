import { afterEach, describe, expect, it } from "vitest"

import type { Today, TodayVerdictKind } from "@/lib/api/today"
import i18n from "@/lib/i18n"
import { mockPlan, mockTodayByVerdict } from "@/test/fixtures"

import { batchBadgeText } from "./format"
import { buildToday, shiftDate, verdictText } from "./today"

describe("lib/plan/today", () => {
  afterEach(async () => {
    await i18n.changeLanguage("en")
  })

  // F5-AC2: each verdict kind renders its exact copy in en and de.
  describe("verdictText", () => {
    const copy: Record<TodayVerdictKind, { en: string; de: string }> = {
      cook: { en: "Cook Chili × 6", de: "Heute kochen: Chili × 6" },
      quick_cook: {
        en: "Quick cook: Omelette",
        de: "Schnell gekocht: Omelette",
      },
      nothing_to_cook: {
        en: "Nothing to cook today",
        de: "Heute wird nicht gekocht",
      },
      no_plates: {
        en: "No plates planned today",
        de: "Heute ist kein Essen geplant",
      },
      no_plan: {
        en: "No plan for today",
        de: "Für heute gibt es noch keinen Plan",
      },
    }
    it.each(Object.keys(copy) as TodayVerdictKind[])("%s", async (kind) => {
      const { verdict } = mockTodayByVerdict[kind]
      await i18n.changeLanguage("en")
      expect(verdictText(verdict, i18n)).toBe(copy[kind].en)
      await i18n.changeLanguage("de")
      expect(verdictText(verdict, i18n)).toBe(copy[kind].de)
    })
  })

  // F5-AC1 (AC-1 step 4): Monday is a leftover day.
  it("Monday: the leftover card 2/3 · use by Wed, then Dinner 3/3", () => {
    const view = buildToday(mockTodayByVerdict.nothing_to_cook, mockPlan, i18n)
    expect(view.headline).toBe("Nothing to cook today")
    expect(view.dateLine).toBe("Monday, November 2")
    expect(view.cook).toEqual([])

    const card = view.leftover
    expect(card).not.toBeNull()
    expect(card?.slotName).toBe("Lunch")
    expect(card?.useBy).toBe("use by Wed")
    expect(card?.portions).toBe(2)
    expect(card && batchBadgeText(card.badge, i18n)).toBe("2/3")

    // The leftover card's meal is not repeated as a row.
    expect(view.rows.map((r) => r.slotName)).toEqual(["Breakfast", "Dinner"])
    const dinner = view.rows[1].cell
    expect(dinner.badge && batchBadgeText(dinner.badge, i18n)).toBe("3/3")
    expect(view.rate?.question).toBe("How was Sunday's Chili?")
  })

  it("a leftover on its use-by day says use by today", () => {
    const today: Today = {
      ...mockTodayByVerdict.nothing_to_cook,
      date: "2026-11-04",
      meals: [],
    }
    const plan = {
      ...mockPlan,
      cells: mockPlan.cells.map((c) =>
        c.date === "2026-11-02" && c.slot_id === 2
          ? { ...c, date: "2026-11-04" }
          : c.date === "2026-11-04" && c.slot_id === 2
            ? { ...c, date: "2026-11-02" }
            : c
      ),
    }
    expect(buildToday(today, plan, i18n).leftover?.useBy).toBe("use by today")
  })

  // F5-AC1 (AC-1 step 5): Sunday is the cook day.
  it("Sunday: Cook Chili × 6 with a cook card and every slot as a row", () => {
    const view = buildToday(mockTodayByVerdict.cook, mockPlan, i18n)
    expect(view.headline).toBe("Cook Chili × 6")
    expect(view.leftover).toBeNull()
    expect(view.cook).toHaveLength(1)
    expect(view.cook[0]).toMatchObject({
      colour: 1,
      slotName: "Dinner",
      minutes: 60,
      coverage: "Covers Sun dinner, Mon lunch, Mon dinner",
    })
    expect(view.rows).toHaveLength(3)
  })

  // F5-AC4: tomorrow eats frozen portions → the defrost reminder.
  it("lists a defrost reminder once per batch", () => {
    const reminder = {
      kind: "defrost" as const,
      session_id: 7,
      food_name: "Chili",
      date: "2026-11-03",
      slot_id: 2,
    }
    const today: Today = {
      ...mockTodayByVerdict.nothing_to_cook,
      reminders: [reminder, { ...reminder, slot_id: 3 }],
    }
    expect(buildToday(today, mockPlan, i18n).reminders).toEqual([
      {
        key: "defrost|7",
        kind: "defrost",
        label: "Freezer",
        text: "Take Chili out of the freezer tonight",
      },
    ])
  })

  it("names the slot in a last-day reminder", () => {
    const today: Today = {
      ...mockTodayByVerdict.nothing_to_cook,
      reminders: [
        {
          kind: "use_by",
          session_id: 7,
          food_name: "Chili",
          date: "2026-11-02",
          slot_id: 3,
        },
      ],
    }
    expect(buildToday(today, mockPlan, i18n).reminders[0]).toMatchObject({
      label: "Last day",
      text: "Dinner is the last of the Chili",
    })
  })

  it("shows the plan banner only when due, and the recipes still missing", () => {
    const due = buildToday(mockTodayByVerdict.no_plan, mockPlan, i18n)
    expect(due.planNext).toBe("2026-11-14")
    expect(due.noPlan).toBe(true)
    const notDue = buildToday(mockTodayByVerdict.no_plates, mockPlan, i18n)
    expect(notDue.planNext).toBeNull()
    expect(notDue.recipesMissing).toBe(0)
    const short = buildToday(
      {
        ...mockTodayByVerdict.no_plates,
        setup: { recipe_count: 7, recipe_target: 10 },
      },
      mockPlan,
      i18n
    )
    expect(short.recipesMissing).toBe(3)
  })

  it("shiftDate crosses months", () => {
    expect(shiftDate("2026-10-31", 1)).toBe("2026-11-01")
    expect(shiftDate("2026-11-01", -1)).toBe("2026-10-31")
  })
})
