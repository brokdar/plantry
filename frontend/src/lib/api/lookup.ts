import { apiFetch } from "./client"
import type { Food } from "./foods"

export interface LookupCandidate {
  name: string
  source_name?: string
  source: string
  barcode: string | null
  fdc_id: number | null
  image_url: string | null
  existing_id: number | null
  kcal_100g: number | null
  protein_100g: number | null
  fat_100g: number | null
  carbs_100g: number | null
  fiber_100g: number | null
  sodium_100g: number | null
  saturated_fat_100g?: number | null
  trans_fat_100g?: number | null
  cholesterol_100g?: number | null
  sugar_100g?: number | null
  potassium_100g?: number | null
  calcium_100g?: number | null
  iron_100g?: number | null
  magnesium_100g?: number | null
  phosphorus_100g?: number | null
  zinc_100g?: number | null
  vitamin_a_100g?: number | null
  vitamin_c_100g?: number | null
  vitamin_d_100g?: number | null
  vitamin_b12_100g?: number | null
  vitamin_b6_100g?: number | null
  folate_100g?: number | null
  portions: { unit: string; grams: number }[]
  serving_quantity_g?: number | null
}

export type TraceLevel = "info" | "success" | "warning" | "error"

export interface TraceEntry {
  step: string
  level: TraceLevel
  summary: string
  duration_ms?: number
  detail?: unknown
}

export interface LookupResponse {
  results: LookupCandidate[]
  recommended_index: number
  trace?: TraceEntry[]
}

export interface LookupParams {
  barcode?: string
  query?: string
  lang?: string
  debug?: boolean
}

export function lookupFoods(params: LookupParams): Promise<LookupResponse> {
  const query = new URLSearchParams()
  if (params.barcode) query.set("barcode", params.barcode)
  if (params.query) query.set("query", params.query)
  if (params.lang) query.set("lang", params.lang)
  if (params.debug) query.set("debug", "true")
  return apiFetch(`/foods/lookup?${query}`)
}

export interface ResolveCandidateInput {
  name: string
  source: string
  barcode?: string | null
  fdc_id?: string | null
  image_url?: string | null
  kcal_100g?: number | null
  protein_100g?: number | null
  fat_100g?: number | null
  carbs_100g?: number | null
  fiber_100g?: number | null
  sodium_100g?: number | null
  saturated_fat_100g?: number | null
  trans_fat_100g?: number | null
  cholesterol_100g?: number | null
  sugar_100g?: number | null
  potassium_100g?: number | null
  calcium_100g?: number | null
  iron_100g?: number | null
  magnesium_100g?: number | null
  phosphorus_100g?: number | null
  zinc_100g?: number | null
  vitamin_a_100g?: number | null
  vitamin_c_100g?: number | null
  vitamin_d_100g?: number | null
  vitamin_b12_100g?: number | null
  vitamin_b6_100g?: number | null
  folate_100g?: number | null
  serving_quantity_g?: number | null
}

export function resolveCandidate(input: ResolveCandidateInput): Promise<Food> {
  return apiFetch(`/foods/resolve`, {
    method: "POST",
    body: JSON.stringify(input),
  })
}
