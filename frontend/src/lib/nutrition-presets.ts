/** Daily nutrition targets as stored on the profile. */
export interface NutritionTargets {
  kcal_target: number | null
  protein_pct: number | null
  fat_pct: number | null
  carbs_pct: number | null
}

export type NutritionPresetId = "cut" | "maintain" | "bulk"

export const NUTRITION_PRESET_IDS: readonly NutritionPresetId[] = [
  "cut",
  "maintain",
  "bulk",
]

/** Plan decision D6. Frontend only: the profile stores just the numbers. */
export const NUTRITION_PRESETS: Record<
  NutritionPresetId,
  {
    kcal_target: number
    protein_pct: number
    fat_pct: number
    carbs_pct: number
  }
> = {
  cut: { kcal_target: 1800, protein_pct: 35, fat_pct: 30, carbs_pct: 35 },
  maintain: { kcal_target: 2200, protein_pct: 30, fat_pct: 30, carbs_pct: 40 },
  bulk: { kcal_target: 2700, protein_pct: 25, fat_pct: 25, carbs_pct: 50 },
}

/** The preset the targets equal, "custom" for any other goal, or null when
 *  no calorie target is set. */
export function matchNutritionPreset(
  targets: NutritionTargets
): NutritionPresetId | "custom" | null {
  if (targets.kcal_target == null) return null
  const match = NUTRITION_PRESET_IDS.find((id) => {
    const p = NUTRITION_PRESETS[id]
    return (
      p.kcal_target === targets.kcal_target &&
      p.protein_pct === targets.protein_pct &&
      p.fat_pct === targets.fat_pct &&
      p.carbs_pct === targets.carbs_pct
    )
  })
  return match ?? "custom"
}

/** Client-side mirror of the server rule: macros add up to at most 100 %. */
export function macroSumValid(targets: NutritionTargets): boolean {
  const sum =
    (targets.protein_pct ?? 0) +
    (targets.fat_pct ?? 0) +
    (targets.carbs_pct ?? 0)
  return sum <= 100
}
