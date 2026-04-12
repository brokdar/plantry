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
  created_at: string
  updated_at: string
}

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
