import { apiFetch } from "./client"

export type DietType = "vegan" | "vegetarian" | "pescetarian" | "omnivore"

export const DIET_TYPES: readonly DietType[] = [
  "vegan",
  "vegetarian",
  "pescetarian",
  "omnivore",
]

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
