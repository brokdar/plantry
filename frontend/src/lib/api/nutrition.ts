import { apiFetch } from "./client"

export interface DayMacros {
  kcal: number
  protein: number
  fat: number
  carbs: number
  fiber: number
  sodium: number
}

export interface NutritionDay {
  date: string
  macros: DayMacros
}

export interface NutritionRangeResponse {
  days: NutritionDay[]
}

export function getNutritionRange(
  from: string,
  to: string
): Promise<NutritionRangeResponse> {
  return apiFetch(
    `/nutrition?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`
  )
}
