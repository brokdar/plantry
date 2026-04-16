import { apiFetch } from "./client"

export interface Profile {
  kcal_target: number | null
  protein_pct: number | null
  fat_pct: number | null
  carbs_pct: number | null
  dietary_restrictions: string[]
  preferences: Record<string, unknown>
  system_prompt: string | null
  locale: string
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
