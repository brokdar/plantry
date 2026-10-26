import { apiFetch } from "./client"
import type { LookupCandidate } from "./lookup"

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

interface FoodBase {
  id: number
  name: string
  source_url?: string | null
  description?: string | null
  image_path: string | null
  favorite: boolean
  last_cooked_at?: string | null
  cook_count: number
  created_at: string
  updated_at: string
}

export interface LeafFood extends FoodBase {
  kind: "leaf"
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
}

export interface ComposedFood extends FoodBase {
  kind: "composed"
  role?: FoodRole | null
  variant_group_id?: number | null
  reference_portions?: number | null
  prep_minutes?: number | null
  cook_minutes?: number | null
  notes?: string | null
  children?: FoodChild[]
  instructions?: FoodInstruction[]
  tags?: string[]
}

export type Food = LeafFood | ComposedFood

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

export interface LeafFoodInput {
  kind: "leaf"
  name: string
  /** Shared. On update, omitting these keeps the stored value. */
  source_url?: string | null
  description?: string | null
  /** Create only: downloaded into the image store after the food is created. */
  image_url?: string
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
}

export interface ComposedFoodInput {
  kind: "composed"
  name: string
  /** Shared. On update, omitting these keeps the stored value. */
  source_url?: string | null
  description?: string | null
  /** Create only: downloaded into the image store after the food is created. */
  image_url?: string
  role?: FoodRole
  reference_portions?: number
  prep_minutes?: number | null
  cook_minutes?: number | null
  notes?: string | null
  children?: FoodChildInput[]
  instructions?: FoodInstructionInput[]
  tags?: string[]
}

export type FoodInput = LeafFoodInput | ComposedFoodInput

/** Leaf payload for an external (FDC/OFF) lookup candidate. Missing per-100 g
 *  values stay null — never 0 — so nutrition completeness can flag them. */
export function leafInputFromCandidate(c: LookupCandidate): LeafFoodInput {
  return {
    kind: "leaf",
    name: c.name,
    source: c.source,
    barcode: c.barcode,
    off_id: c.source === "off" ? c.barcode : null,
    fdc_id: c.fdc_id != null ? String(c.fdc_id) : null,
    kcal_100g: c.kcal_100g,
    protein_100g: c.protein_100g,
    fat_100g: c.fat_100g,
    carbs_100g: c.carbs_100g,
    fiber_100g: c.fiber_100g,
    sodium_100g: c.sodium_100g,
    saturated_fat_100g: c.saturated_fat_100g ?? null,
    trans_fat_100g: c.trans_fat_100g ?? null,
    cholesterol_100g: c.cholesterol_100g ?? null,
    sugar_100g: c.sugar_100g ?? null,
    potassium_100g: c.potassium_100g ?? null,
    calcium_100g: c.calcium_100g ?? null,
    iron_100g: c.iron_100g ?? null,
    magnesium_100g: c.magnesium_100g ?? null,
    phosphorus_100g: c.phosphorus_100g ?? null,
    zinc_100g: c.zinc_100g ?? null,
    vitamin_a_100g: c.vitamin_a_100g ?? null,
    vitamin_c_100g: c.vitamin_c_100g ?? null,
    vitamin_d_100g: c.vitamin_d_100g ?? null,
    vitamin_b12_100g: c.vitamin_b12_100g ?? null,
    vitamin_b6_100g: c.vitamin_b6_100g ?? null,
    folate_100g: c.folate_100g ?? null,
  }
}

export interface FoodListParams {
  kind?: FoodKind
  search?: string
  role?: FoodRole
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

/** Non-fatal problems hit while creating a food. */
export type FoodCreateWarning = "image_download_failed"

/** POST /api/foods. A duplicate leaf name rejects with 409
 *  `{ message_key: "error.food.duplicate_name", status: 409, existing_id }`. */
export function createFood(
  input: FoodInput
): Promise<Food & { warnings?: FoodCreateWarning[] }> {
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

/** A leaf whose kcal/protein/fat/carbs are missing, so the recipe total
 *  under-counts. */
export interface MissingNutritionLeaf {
  id: number
  name: string
}

/** GET /api/foods/:id/nutrition — per-portion macros for composed foods.
 *  `complete` is false when any leaf lacks core macros; `missing` lists them
 *  (`[]` when complete). */
export interface FoodNutritionDetail extends FoodNutrition {
  complete: boolean
  missing: MissingNutritionLeaf[]
}

export function getFoodNutrition(id: number): Promise<FoodNutritionDetail> {
  return apiFetch(`/foods/${id}/nutrition`)
}

export interface FoodMacrosEntry {
  food_id: number
  macros: FoodNutrition
}

/** GET /api/foods/macros?ids=1,2,3 — batch per-portion (composed) or per-100g
 *  (leaf) macros. Unknown ids are silently omitted. Returns `{ foods: [] }`
 *  when given no resolvable ids. Throws on empty/missing `ids` (400). */
export function listFoodMacros(
  ids: readonly number[]
): Promise<{ foods: FoodMacrosEntry[] }> {
  return apiFetch(`/foods/macros?ids=${ids.join(",")}`)
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
  role?: FoodRole | null
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

/** GET /api/foods/:id/recent-unit — most-recently-used unit for a leaf food
 *  on any plate, or null if the food has never been added with a unit. */
export function getRecentUnit(id: number): Promise<{ unit: string | null }> {
  return apiFetch(`/foods/${id}/recent-unit`)
}
