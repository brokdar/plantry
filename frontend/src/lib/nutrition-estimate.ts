import type { ActivityLevel, Goal, Sex } from "@/lib/api/profile"

export interface BodyData {
  weight_kg: number | null
  height_cm: number | null
  birth_year: number | null
  sex: Sex | null
  activity_level: ActivityLevel | null
}

export interface KcalEstimate {
  /** Resting energy (Mifflin-St Jeor), kcal/day. */
  rest: number
  /** Rest x activity level, kcal/day. */
  maintenance: number
  /** The suggested target for the goal. */
  target: number
}

/** Lowest unsupervised daily kcal (mirrors the backend's MinDailyKcal). */
export function minDailyKcal(sex: Sex | null): number {
  return sex === "male" ? 1500 : 1200
}

/** The estimate needs weight, height, birth year and an activity level. */
export function canEstimate(b: BodyData): boolean {
  return (
    b.weight_kg != null &&
    b.height_cm != null &&
    b.birth_year != null &&
    b.activity_level != null
  )
}

/**
 * Estimated daily kcal: Mifflin-St Jeor resting energy x PAL (DGE: energy
 * need = resting energy x physical activity level). An estimate, good to
 * about +-10-20 %. Without a sex the two constants are averaged. Losing
 * subtracts 500 kcal (at most 25 %), gaining adds 300; the result never goes
 * below the safety floor of a known sex. Returns null while data is missing.
 */
export function estimateKcal(
  b: BodyData,
  goal: Goal,
  year = new Date().getFullYear()
): KcalEstimate | null {
  if (!canEstimate(b)) return null
  const age = year - b.birth_year!
  const sexTerm = b.sex === "male" ? 5 : b.sex === "female" ? -161 : -78
  const rest = 10 * b.weight_kg! + 6.25 * b.height_cm! - 5 * age + sexTerm
  const maintenance = rest * b.activity_level!
  let target = maintenance
  if (goal === "lose") target -= Math.min(500, maintenance * 0.25)
  if (goal === "gain") target += 300
  target = Math.max(target, minDailyKcal(b.sex))
  const round10 = (v: number) => Math.round(v / 10) * 10
  return {
    rest: Math.round(rest),
    maintenance: round10(maintenance),
    target: round10(target),
  }
}
