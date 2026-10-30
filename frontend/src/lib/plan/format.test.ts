import { afterEach, beforeEach, describe, expect, it } from "vitest"

import i18n from "@/lib/i18n"
import { mockSession } from "@/test/fixtures"

import {
  coverageText,
  formatAmount,
  formatQuantity,
  relativeDay,
  weekdayShort,
} from "./format"

const slotNames: Record<number, string> = {
  1: "Breakfast",
  2: "Lunch",
  3: "Dinner",
}
const slotName = (id: number) => slotNames[id]

describe("lib/plan/format", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("en")
  })
  afterEach(async () => {
    await i18n.changeLanguage("en")
  })

  describe("formatAmount", () => {
    it.each([
      [1, "1"],
      [0.25, "¼"],
      [0.5, "½"],
      [1.5, "1½"],
      [2 / 3, "⅔"],
      [1 + 1 / 3, "1⅓"],
      [2.75, "2¾"],
      [1.2, "1.2"],
      [1840, "1,840"],
    ])("%s → %s", (n, out) => {
      expect(formatAmount(n, i18n)).toBe(out)
    })

    it("uses the locale's decimal and grouping marks", async () => {
      await i18n.changeLanguage("de")
      expect(formatAmount(1.2, i18n)).toBe("1,2")
      expect(formatAmount(1840, i18n)).toBe("1.840")
      expect(formatAmount(3.5, i18n)).toBe("3½")
    })
  })

  describe("formatQuantity", () => {
    it('renders 1.5 piece as "1½ pcs"', () => {
      expect(formatQuantity(1.5, "piece", i18n)).toBe("1½ pcs")
    })

    it("renders count fractions in German", async () => {
      await i18n.changeLanguage("de")
      expect(formatQuantity(1.5, "piece", i18n)).toBe("1½ Stk.")
      expect(formatQuantity(0.5, "tbsp", i18n)).toBe("½ EL")
    })

    it("keeps mass and volume decimal", () => {
      expect(formatQuantity(600, "g", i18n)).toBe("600 g")
      expect(formatQuantity(1.5, "kg", i18n)).toBe("1.5 kg")
      expect(formatQuantity(1.5, "tsp", i18n)).toBe("1½ tsp")
    })

    it("falls back to the unit's name for count units without a short form", () => {
      expect(formatQuantity(2, "clove", i18n)).toBe("2 cloves")
      expect(formatQuantity(1, "clove", i18n)).toBe("1 clove")
    })
  })

  describe("coverageText", () => {
    it("lists every linked cell in order", () => {
      expect(coverageText(mockSession.links, slotName, i18n)).toBe(
        "Covers Sun dinner, Mon lunch, Mon dinner"
      )
    })

    it("keeps German nouns capitalised", async () => {
      await i18n.changeLanguage("de")
      const de: Record<number, string> = {
        2: "Mittagessen",
        3: "Abendessen",
      }
      expect(coverageText(mockSession.links, (id) => de[id], i18n)).toMatch(
        /^Reicht für So\.? Abendessen, Mo\.? Mittagessen, Mo\.? Abendessen$/
      )
    })
  })

  describe("weekdayShort", () => {
    it("names the weekday of a calendar date in the active locale", async () => {
      expect(weekdayShort("2026-11-01", i18n)).toBe("Sun")
      // ICU versions differ on the period.
      await i18n.changeLanguage("de")
      expect(weekdayShort("2026-11-01", i18n)).toMatch(/^So\.?$/)
    })
  })

  describe("relativeDay", () => {
    it.each([
      ["2026-11-02", "today"],
      ["2026-11-03", "tomorrow"],
      ["2026-11-01", "yesterday"],
      ["2026-11-05", "in 3 days"],
      ["2026-10-30", "3 days ago"],
    ])("%s relative to 2026-11-02 → %s", (date, out) => {
      expect(relativeDay(date, "2026-11-02", i18n)).toBe(out)
    })

    it("counts calendar days across a DST change", () => {
      // Central Europe switches back on 2026-10-25.
      expect(relativeDay("2026-10-26", "2026-10-24", i18n)).toBe("in 2 days")
    })

    it("speaks German", async () => {
      await i18n.changeLanguage("de")
      expect(relativeDay("2026-11-03", "2026-11-02", i18n)).toBe("morgen")
      expect(relativeDay("2026-10-30", "2026-11-02", i18n)).toBe("vor 3 Tagen")
    })
  })
})
