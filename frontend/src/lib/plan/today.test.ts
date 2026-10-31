import { afterEach, describe, expect, it } from "vitest"

import type { Today, TodayVerdictKind } from "@/lib/api/today"
import i18n from "@/lib/i18n"
import { mockPlan, mockTodayByVerdict } from "@/test/fixtures"

import { batchBadgeText } from "./format"
import {
  buildTodayHeader,
  buildTodayPlan,
  shiftDate,
  verdictText,
} from "./today"

const SUNDAY = "2026-11-01"
const MONDAY = "2026-11-02"

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
        de: "Heute schnell: Omelette",
      },
      nothing_to_cook: {
        en: "Nothing to cook today",
        de: "Heute wird nicht gekocht",
      },
      draft_only: {
        en: "Your plan for this week is still a draft",
        de: "Dein Plan für diese Woche ist noch ein Entwurf",
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

    // Another day than today names its weekday instead of saying "today".
    const otherDay: Record<TodayVerdictKind, { en: string; de: string }> = {
      cook: {
        en: "Cook Chili × 6 on Sunday",
        de: "Am Sonntag kochen: Chili × 6",
      },
      quick_cook: {
        en: "Quick cook on Sunday: Omelette",
        de: "Am Sonntag schnell gekocht: Omelette",
      },
      nothing_to_cook: {
        en: "Nothing to cook on Sunday",
        de: "Am Sonntag wird nicht gekocht",
      },
      draft_only: {
        en: "The plan for Sunday is still a draft",
        de: "Der Plan für Sonntag ist noch ein Entwurf",
      },
      no_plates: {
        en: "No plates planned for Sunday",
        de: "Für Sonntag ist kein Essen geplant",
      },
      no_plan: {
        en: "No plan for Sunday yet",
        de: "Für Sonntag gibt es noch keinen Plan",
      },
    }
    it.each(Object.keys(otherDay) as TodayVerdictKind[])(
      "%s on another day",
      async (kind) => {
        const today = { ...mockTodayByVerdict[kind], date: SUNDAY }
        await i18n.changeLanguage("en")
        expect(buildTodayHeader(today, MONDAY, i18n).headline).toBe(
          otherDay[kind].en
        )
        await i18n.changeLanguage("de")
        expect(buildTodayHeader(today, MONDAY, i18n).headline).toBe(
          otherDay[kind].de
        )
      }
    )
  })

  // F5-AC1 (AC-1 step 4): Monday is a leftover day.
  it("Monday: the leftover card 2/3 · use by Wed, then Dinner 3/3", () => {
    const today = mockTodayByVerdict.nothing_to_cook
    const header = buildTodayHeader(today, MONDAY, i18n)
    expect(header.headline).toBe("Nothing to cook today")
    expect(header.dateLine).toBe("Monday, November 2")
    expect(header.rate?.question).toBe("How was Sunday's Chili?")

    const view = buildTodayPlan(today, mockPlan, MONDAY, i18n)
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
  })

  describe("use-by on the leftover card", () => {
    // Monday's lunch moved to Wednesday 11-04, the batch's use-by day.
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

    it("says use by today on its last day when that is the real today", () => {
      expect(
        buildTodayPlan(today, plan, "2026-11-04", i18n).leftover?.useBy
      ).toBe("use by today")
    })

    it("names the weekday when that day is shown from another day", () => {
      expect(
        buildTodayPlan(today, plan, "2026-11-01", i18n).leftover?.useBy
      ).toBe("use by Wed")
    })
  })

  // F5-AC1 (AC-1 step 5): Sunday is the cook day.
  it("Sunday: Cook Chili × 6 with a cook card and every slot as a row", () => {
    const today = mockTodayByVerdict.cook
    expect(buildTodayHeader(today, SUNDAY, i18n).headline).toBe(
      "Cook Chili × 6"
    )
    const view = buildTodayPlan(today, mockPlan, SUNDAY, i18n)
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

  // DoD walkthrough: a recipe saved with 0 minutes has no time estimate.
  it("treats 0 minutes as unknown on the cook card", () => {
    const cook = mockTodayByVerdict.cook
    const today: Today = {
      ...cook,
      cook: cook.cook.map((c) => ({ ...c, minutes: 0 })),
    }
    expect(buildTodayPlan(today, mockPlan, SUNDAY, i18n).cook[0].minutes).toBe(
      null
    )
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
    expect(buildTodayPlan(today, mockPlan, MONDAY, i18n).reminders).toEqual([
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
    expect(
      buildTodayPlan(today, mockPlan, MONDAY, i18n).reminders[0]
    ).toMatchObject({
      label: "Last day",
      text: "Dinner is the last of the Chili",
    })
  })

  it("asks the rating question only on or before the real today", () => {
    const today = mockTodayByVerdict.nothing_to_cook
    expect(buildTodayHeader(today, MONDAY, i18n).rate).not.toBeNull()
    expect(buildTodayHeader(today, "2026-11-05", i18n).rate).not.toBeNull()
    expect(buildTodayHeader(today, SUNDAY, i18n).rate).toBeNull()
  })

  it("shows the plan banner only when due, and the recipes still missing", () => {
    const due = buildTodayHeader(mockTodayByVerdict.no_plan, MONDAY, i18n)
    expect(due.planNext).toBe("2026-11-14")
    expect(due.noPlan).toBe(true)
    const notDue = buildTodayHeader(mockTodayByVerdict.no_plates, MONDAY, i18n)
    expect(notDue.planNext).toBeNull()
    expect(notDue.recipesMissing).toBe(0)
    const short = buildTodayHeader(
      {
        ...mockTodayByVerdict.no_plates,
        setup: { recipe_count: 7, recipe_target: 10 },
      },
      MONDAY,
      i18n
    )
    expect(short.recipesMissing).toBe(3)
  })

  it("shiftDate crosses months", () => {
    expect(shiftDate("2026-10-31", 1)).toBe("2026-11-01")
    expect(shiftDate("2026-11-01", -1)).toBe("2026-10-31")
  })
})
