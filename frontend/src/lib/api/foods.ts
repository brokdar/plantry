import { apiFetch } from "./client"

export type FoodKind = "leaf" | "composed"
export type FoodSource = "manual" | "off" | "fdc"
export type FoodRole =
  | "main"
  | "side_starch"
  | "side_veg"
  | "side_protein"
  | "sauce"
  | "drink"
  | "dessert"
  | "standalone"

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

export interface FoodPortion {
  food_id: number
  unit: string
  grams: number
}

export interface FoodChild {
  id: number
  parent_id: number
  child_id: number
  child_name: string
  child_kind: FoodKind
  amount: number
  unit: string
  grams: number
  grams_source?: string
  sort_order: number
}

export interface FoodInstruction {
  id: number
  food_id: number
  step_number: number
  text: string
}

export interface Food {
  id: number
  kind: FoodKind
  name: string

  source?: FoodSource | null
  barcode?: string | null
  off_id?: string | null
  fdc_id?: string | null
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
  portions?: FoodPortion[]

  role?: FoodRole | null
  variant_group_id?: number | null
  reference_portions?: number | null
  prep_minutes?: number | null
  cook_minutes?: number | null
  notes?: string | null
  children?: FoodChild[]
  instructions?: FoodInstruction[]
  tags?: string[]

  image_path: string | null
  favorite: boolean
  last_cooked_at?: string | null
  cook_count: number
  created_at: string
  updated_at: string
}

export interface FoodListResponse {
  items: Food[]
  total: number
}

export interface FoodChildInput {
  child_id: number
  amount: number
  unit: string
  grams: number
  sort_order: number
}

export interface FoodInstructionInput {
  step_number: number
  text: string
}

export interface FoodInput {
  name: string
  kind: FoodKind

  // leaf
  source?: string
  barcode?: string | null
  off_id?: string | null
  fdc_id?: string | null
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

  // composed
  role?: string
  reference_portions?: number
  prep_minutes?: number | null
  cook_minutes?: number | null
  notes?: string | null
  children?: FoodChildInput[]
  instructions?: FoodInstructionInput[]
  tags?: string[]
}

export interface FoodListParams {
  kind?: FoodKind
  search?: string
  role?: string
  tag?: string
  favorite?: 0 | 1
  limit?: number
  offset?: number
  sort?: string
  order?: string
}

export function listFoods(params?: FoodListParams): Promise<FoodListResponse> {
  const query = new URLSearchParams()
  if (params) {
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined) {
        query.set(key, String(value))
      }
    })
  }
  const qs = query.toString()
  return apiFetch(`/foods${qs ? `?${qs}` : ""}`)
}

export function getFood(id: number): Promise<Food> {
  return apiFetch(`/foods/${id}`)
}

export function createFood(input: FoodInput): Promise<Food> {
  return apiFetch("/foods", {
    method: "POST",
    body: JSON.stringify(input),
  })
}

export function updateFood(id: number, input: FoodInput): Promise<Food> {
  return apiFetch(`/foods/${id}`, {
    method: "PUT",
    body: JSON.stringify(input),
  })
}

export function deleteFood(id: number): Promise<void> {
  return apiFetch(`/foods/${id}`, { method: "DELETE" })
}

export interface FoodNutrition {
  kcal: number
  protein: number
  fat: number
  carbs: number
  fiber: number
  sodium: number
}

export function getFoodNutrition(id: number): Promise<FoodNutrition> {
  return apiFetch(`/foods/${id}/nutrition`)
}

export function setFoodFavorite(id: number, favorite: boolean): Promise<Food> {
  return apiFetch(`/foods/${id}/favorite`, {
    method: "POST",
    body: JSON.stringify({ favorite }),
  })
}

export interface VariantListResponse {
  items: Food[]
}

export function createVariant(id: number): Promise<Food> {
  return apiFetch(`/foods/${id}/variant`, { method: "POST" })
}

export function listVariants(id: number): Promise<VariantListResponse> {
  return apiFetch(`/foods/${id}/variants`)
}

export interface FoodSummary {
  id: number
  name: string
  role?: string | null
  image_path?: string | null
  cook_count: number
  last_cooked_at?: string | null
}

export interface InsightsResponse {
  forgotten: FoodSummary[]
  most_cooked: FoodSummary[]
}

export interface InsightsParams {
  forgotten_weeks?: number
  forgotten_limit?: number
  most_cooked_limit?: number
}

export function getInsights(
  params?: InsightsParams
): Promise<InsightsResponse> {
  const query = new URLSearchParams()
  if (params) {
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined) {
        query.set(key, String(value))
      }
    })
  }
  const qs = query.toString()
  return apiFetch(`/foods/insights${qs ? `?${qs}` : ""}`)
}

// ── Portions ──────────────────────────────────────────────────────────

export function listPortions(
  foodId: number
): Promise<{ items: FoodPortion[] }> {
  return apiFetch(`/foods/${foodId}/portions`)
}

export function upsertPortion(
  foodId: number,
  data: { unit: string; grams: number }
): Promise<FoodPortion> {
  return apiFetch(`/foods/${foodId}/portions`, {
    method: "POST",
    body: JSON.stringify(data),
  })
}

export function deletePortion(foodId: number, unit: string): Promise<void> {
  return apiFetch(`/foods/${foodId}/portions/${encodeURIComponent(unit)}`, {
    method: "DELETE",
  })
}

export interface SyncPortionsResponse {
  added: number
  portions: FoodPortion[]
}

export function syncPortions(id: number): Promise<SyncPortionsResponse> {
  return apiFetch(`/foods/${id}/sync-portions`, { method: "POST" })
}

export function refetchFood(id: number, lang?: string): Promise<Food> {
  const qs = lang ? `?lang=${encodeURIComponent(lang)}` : ""
  return apiFetch(`/foods/${id}/refetch${qs}`, { method: "POST" })
}
