import { apiFetch } from "./client"

export interface LookupCandidate {
  name: string
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
  portions: { unit: string; grams: number }[]
}

export interface LookupResponse {
  results: LookupCandidate[]
  recommended_index: number
}

export interface LookupParams {
  barcode?: string
  query?: string
  lang?: string
}

export function lookupIngredients(
  params: LookupParams
): Promise<LookupResponse> {
  const query = new URLSearchParams()
  if (params.barcode) query.set("barcode", params.barcode)
  if (params.query) query.set("query", params.query)
  if (params.lang) query.set("lang", params.lang)
  return apiFetch(`/ingredients/lookup?${query}`)
}
