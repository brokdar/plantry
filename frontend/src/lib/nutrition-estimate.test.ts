import { describe, expect, it } from "vitest"

import {
  canEstimate,
  estimateKcal,
  minDailyKcal,
  type BodyData,
} from "./nutrition-estimate"

const male: BodyData = {
  weight_kg: 80,
  height_cm: 180,
  birth_year: 1986,
  sex: "male",
  activity_level: 1.6,
}

describe("estimateKcal", () => {
  it("is null while body data is missing", () => {
    expect(canEstimate({ ...male, weight_kg: null })).toBe(false)
    expect(
      estimateKcal({ ...male, activity_level: null }, "maintain")
    ).toBeNull()
  })

  it("uses Mifflin-St Jeor x PAL (male, 40 years)", () => {
    // 10*80 + 6.25*180 - 5*40 + 5 = 1730; x1.6 = 2768
    const e = estimateKcal(male, "maintain", 2026)
    expect(e).toEqual({ rest: 1730, maintenance: 2770, target: 2770 })
  })

  it("subtracts for lose and adds for gain", () => {
    expect(estimateKcal(male, "lose", 2026)?.target).toBe(2270)
    expect(estimateKcal(male, "gain", 2026)?.target).toBe(3070)
  })

  it("averages the sex constants without a sex", () => {
    // 800 + 1125 - 200 - 78 = 1647; x1.4 = 2305.8
    const e = estimateKcal(
      {
        weight_kg: 80,
        height_cm: 180,
        birth_year: 1986,
        sex: null,
        activity_level: 1.4,
      },
      "maintain",
      2026
    )
    expect(e?.maintenance).toBe(2310)
  })

  it("never suggests below the floor of a known sex", () => {
    const small: BodyData = {
      weight_kg: 45,
      height_cm: 150,
      birth_year: 1946,
      sex: "female",
      activity_level: 1.4,
    }
    expect(estimateKcal(small, "lose", 2026)?.target).toBe(1200)
  })

  it("loses at most 25 % of maintenance", () => {
    const e = estimateKcal(
      { ...male, weight_kg: 50, height_cm: 150, activity_level: 1.4 },
      "lose",
      2026
    )
    expect(e!.target).toBeGreaterThanOrEqual(e!.maintenance * 0.75 - 10)
  })
})

describe("minDailyKcal", () => {
  it("matches the backend floors", () => {
    expect(minDailyKcal("female")).toBe(1200)
    expect(minDailyKcal("male")).toBe(1500)
    expect(minDailyKcal(null)).toBe(1200)
  })
})
