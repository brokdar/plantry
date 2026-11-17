import { apiFetch } from "./client"

export type DietType = "vegan" | "vegetarian" | "pescetarian" | "omnivore"

export const DIET_TYPES: readonly DietType[] = [
  "vegan",
  "vegetarian",
  "pescetarian",
  "omnivore",
]

export type Goal = "maintain" | "lose" | "gain"
export const GOALS: readonly Goal[] = ["maintain", "lose", "gain"]
export type Sex = "female" | "male"
/** PAL: mostly sitting, standing, physically active. */
export type ActivityLevel = 1.4 | 1.6 | 1.8
export const ACTIVITY_LEVELS: readonly ActivityLevel[] = [1.4, 1.6, 1.8]

export interface Profile {
  kcal_target: number | null
  protein_pct: number | null
  fat_pct: number | null
  carbs_pct: number | null
  dietary_restrictions: string[]
  preferences: Record<string, unknown>
  system_prompt: string | null
  locale: string
  diet_type: DietType
  /** 0–7, or null for no limit. */
  meat_days_per_week_max: number | null
  /** Portions per planned plate (> 0, ≤ 20). */
  household_portions: number
  goal: Goal
  /** Optional body data; they feed the kcal estimate and the safety floor. */
  weight_kg: number | null
  /** Optional protein factor in g per kg body weight (0.8–2.2). */
  protein_g_per_kg: number | null
  height_cm: number | null
  birth_year: number | null
  sex: Sex | null
  activity_level: ActivityLevel | null
  updated_at: string
}

export interface ProfileInput {
  kcal_target?: number | null
  protein_pct?: number | null
  fat_pct?: number | null
  carbs_pct?: number | null
  dietary_restrictions?: string[]
  preferences?: Record<string, unknown>
  system_prompt?: string | null
  locale?: string
  diet_type?: DietType
  meat_days_per_week_max?: number | null
  household_portions?: number
  goal?: Goal
  weight_kg?: number | null
  protein_g_per_kg?: number | null
  height_cm?: number | null
  birth_year?: number | null
  sex?: Sex | null
  activity_level?: ActivityLevel | null
}

/** The full PUT body for a stored profile. `PUT /api/profile` is a full
 *  replace, so every write starts from this. */
export function profileToInput(profile: Profile): ProfileInput {
  return {
    kcal_target: profile.kcal_target,
    protein_pct: profile.protein_pct,
    fat_pct: profile.fat_pct,
    carbs_pct: profile.carbs_pct,
    dietary_restrictions: profile.dietary_restrictions ?? [],
    preferences: profile.preferences,
    system_prompt: profile.system_prompt,
    locale: profile.locale,
    diet_type: profile.diet_type,
    meat_days_per_week_max: profile.meat_days_per_week_max,
    household_portions: profile.household_portions,
    goal: profile.goal,
    weight_kg: profile.weight_kg,
    protein_g_per_kg: profile.protein_g_per_kg,
    height_cm: profile.height_cm,
    birth_year: profile.birth_year,
    sex: profile.sex,
    activity_level: profile.activity_level,
  }
}

export function getProfile(): Promise<Profile> {
  return apiFetch("/profile")
}

export function updateProfile(input: ProfileInput): Promise<Profile> {
  return apiFetch("/profile", {
    method: "PUT",
    body: JSON.stringify(input),
  })
}
