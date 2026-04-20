import { apiFetch } from "./client"

export interface Ingredient {
  id: number
  name: string
  source: string
  barcode: string | null
  off_id: string | null
  fdc_id: string | null
  image_path: string | null
  kcal_100g: number
  protein_100g: number
  fat_100g: number
  carbs_100g: number
  fiber_100g: number
  sodium_100g: number
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
  created_at: string
  updated_at: string
}

export const EXTENDED_NUTRIENT_KEYS = [
  "saturated_fat_100g",
  "trans_fat_100g",
  "cholesterol_100g",
  "sugar_100g",
  "potassium_100g",
  "calcium_100g",
  "iron_100g",
  "magnesium_100g",
  "phosphorus_100g",
  "zinc_100g",
  "vitamin_a_100g",
  "vitamin_c_100g",
  "vitamin_d_100g",
  "vitamin_b12_100g",
  "vitamin_b6_100g",
  "folate_100g",
] as const

export type ExtendedNutrientKey = (typeof EXTENDED_NUTRIENT_KEYS)[number]

export interface IngredientListResponse {
  items: Ingredient[]
  total: number
}

export interface IngredientInput {
  name: string
  source?: string
  barcode?: string | null
  off_id?: string | null
  fdc_id?: string | null
  kcal_100g?: number
  protein_100g?: number
  fat_100g?: number
  carbs_100g?: number
  fiber_100g?: number
  sodium_100g?: number
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
}

export interface IngredientListParams {
  search?: string
  limit?: number
  offset?: number
  sort?: string
  order?: string
}

export function listIngredients(
  params?: IngredientListParams
): Promise<IngredientListResponse> {
  const query = new URLSearchParams()
  if (params) {
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined) {
        query.set(key, String(value))
      }
    })
  }
  const qs = query.toString()
  return apiFetch(`/ingredients${qs ? `?${qs}` : ""}`)
}

export function getIngredient(id: number): Promise<Ingredient> {
  return apiFetch(`/ingredients/${id}`)
}

export function createIngredient(input: IngredientInput): Promise<Ingredient> {
  return apiFetch("/ingredients", {
    method: "POST",
    body: JSON.stringify(input),
  })
}

export function updateIngredient(
  id: number,
  input: IngredientInput
): Promise<Ingredient> {
  return apiFetch(`/ingredients/${id}`, {
    method: "PUT",
    body: JSON.stringify(input),
  })
}

export function deleteIngredient(id: number): Promise<void> {
  return apiFetch(`/ingredients/${id}`, {
    method: "DELETE",
  })
}

/** refetchIngredient re-queries the upstream source (OFF or FDC) using the
 *  stored barcode/fdc_id and replaces the ingredient's nutrient fields with
 *  fresh values. Returns the updated ingredient. */
export function refetchIngredient(
  id: number,
  lang?: string
): Promise<Ingredient> {
  const qs = lang ? `?lang=${encodeURIComponent(lang)}` : ""
  return apiFetch(`/ingredients/${id}/refetch${qs}`, {
    method: "POST",
  })
}
