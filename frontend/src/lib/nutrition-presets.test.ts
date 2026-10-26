import { describe, expect, it } from "vitest"

import {
  NUTRITION_PRESETS,
  NUTRITION_PRESET_IDS,
  macroSumValid,
  matchNutritionPreset,
} from "./nutrition-presets"

describe("nutrition presets (D6)", () => {
  it("defines cut, maintain and bulk with the D6 numbers", () => {
    expect(NUTRITION_PRESET_IDS).toEqual(["cut", "maintain", "bulk"])
    expect(NUTRITION_PRESETS.cut).toEqual({
      kcal_target: 1800,
      protein_pct: 35,
      fat_pct: 30,
      carbs_pct: 35,
    })
    expect(NUTRITION_PRESETS.maintain).toEqual({
      kcal_target: 2200,
      protein_pct: 30,
      fat_pct: 30,
      carbs_pct: 40,
    })
    expect(NUTRITION_PRESETS.bulk).toEqual({
      kcal_target: 2700,
      protein_pct: 25,
      fat_pct: 25,
      carbs_pct: 50,
    })
  })

  it("matches stored targets to a preset", () => {
    expect(matchNutritionPreset(NUTRITION_PRESETS.bulk)).toBe("bulk")
  })

  it("reports other targets as custom", () => {
    expect(
      matchNutritionPreset({
        kcal_target: 2000,
        protein_pct: 30,
        fat_pct: 30,
        carbs_pct: 40,
      })
    ).toBe("custom")
  })

  it("reports no goal when the calorie target is unset", () => {
    expect(
      matchNutritionPreset({
        kcal_target: null,
        protein_pct: null,
        fat_pct: null,
        carbs_pct: null,
      })
    ).toBeNull()
  })

  it("accepts macro sums up to 100 %", () => {
    const base = { kcal_target: 2000, protein_pct: 40, fat_pct: 30 }
    expect(macroSumValid({ ...base, carbs_pct: 30 })).toBe(true)
    expect(macroSumValid({ ...base, carbs_pct: 31 })).toBe(false)
  })
})
