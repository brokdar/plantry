import type { ComposedFood, LeafFood } from "@/lib/api/foods"
import type { LookupCandidate, LookupResponse } from "@/lib/api/lookup"
import type { Plate } from "@/lib/api/plates"
import type {
  ShoppingListResponse,
  WeekNutritionResponse,
} from "@/lib/api/weeks"

export const mockChickenBreast: LeafFood = {
  id: 1,
  kind: "leaf",
  name: "Chicken breast",
  source: "manual",
  barcode: null,
  off_id: null,
  fdc_id: null,
  image_path: null,
  favorite: false,
  cook_count: 0,
  kcal_100g: 165,
  protein_100g: 31,
  fat_100g: 3.6,
  carbs_100g: 0,
  fiber_100g: 0,
  sodium_100g: 0,
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
}

export const mockBrownRice: LeafFood = {
  id: 2,
  kind: "leaf",
  name: "Brown rice",
  source: "manual",
  barcode: null,
  off_id: null,
  fdc_id: null,
  image_path: null,
  favorite: false,
  cook_count: 0,
  kcal_100g: 112,
  protein_100g: 2.3,
  fat_100g: 0.8,
  carbs_100g: 24,
  fiber_100g: 1.8,
  sodium_100g: 1,
  created_at: "2024-01-02T00:00:00Z",
  updated_at: "2024-01-02T00:00:00Z",
}

export const mockChickenCurry: ComposedFood = {
  id: 3,
  kind: "composed",
  name: "Chicken Curry",
  role: "main",
  variant_group_id: null,
  reference_portions: 2,
  prep_minutes: 10,
  cook_minutes: 30,
  image_path: null,
  notes: null,
  last_cooked_at: null,
  cook_count: 0,
  favorite: false,
  children: [
    {
      id: 1,
      parent_id: 3,
      child_id: 1,
      child_name: "Chicken breast",
      child_kind: "leaf",
      amount: 300,
      unit: "g",
      grams: 300,
      sort_order: 0,
    },
  ],
  instructions: [
    { id: 1, food_id: 3, step_number: 1, text: "Cook chicken" },
    { id: 2, food_id: 3, step_number: 2, text: "Add curry paste" },
  ],
  tags: ["spicy", "thai"],
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
}

export const mockTofuBowl: ComposedFood = {
  id: 4,
  kind: "composed",
  name: "Tofu Bowl",
  role: "standalone",
  variant_group_id: null,
  reference_portions: 1,
  prep_minutes: 5,
  cook_minutes: 15,
  image_path: null,
  notes: null,
  last_cooked_at: null,
  cook_count: 0,
  favorite: false,
  children: [],
  instructions: [],
  tags: ["vegan"],
  created_at: "2024-01-02T00:00:00Z",
  updated_at: "2024-01-02T00:00:00Z",
}

export const mockChickenCurryWithVariantGroup: ComposedFood = {
  ...mockChickenCurry,
  id: 10,
  variant_group_id: 1,
}

export const mockTofuCurryVariant: ComposedFood = {
  id: 11,
  kind: "composed",
  name: "Tofu Curry",
  role: "main",
  variant_group_id: 1,
  reference_portions: 2,
  prep_minutes: 10,
  cook_minutes: 25,
  image_path: null,
  notes: null,
  last_cooked_at: null,
  cook_count: 0,
  favorite: false,
  children: [],
  instructions: [],
  tags: ["vegan"],
  created_at: "2024-01-03T00:00:00Z",
  updated_at: "2024-01-03T00:00:00Z",
}

export const mockShoppingList: ShoppingListResponse = {
  items: [
    { food_id: 1, name: "Chicken", total_grams: 100 },
    { food_id: 2, name: "Rice", total_grams: 200 },
  ],
}

export const mockWeekNutrition: WeekNutritionResponse = {
  days: [
    {
      day: 0,
      macros: {
        kcal: 500,
        protein: 40,
        fat: 15,
        carbs: 50,
        fiber: 5,
        sodium: 1,
      },
    },
  ],
  week: { kcal: 500, protein: 40, fat: 15, carbs: 50, fiber: 5, sodium: 1 },
}

export const mockLookupCandidate: LookupCandidate = {
  name: "Chicken Breast, Raw",
  source: "fdc",
  barcode: null,
  fdc_id: 171077,
  image_url: null,
  existing_id: null,
  kcal_100g: 120,
  protein_100g: 22.5,
  fat_100g: 2.6,
  carbs_100g: 0,
  fiber_100g: 0,
  sodium_100g: 0.074,
  portions: [],
}

export const mockLookupResponse: LookupResponse = {
  results: [mockLookupCandidate],
  recommended_index: 0,
}

// Plates spanning two ISO weeks (2026-W16 and 2026-W17)
// 2026-04-13 = Mon W16, 2026-04-14 = Tue W16, 2026-04-20 = Mon W17
export const mockPlateW16a: Plate = {
  id: 101,
  week_id: 1,
  day: 0,
  slot_id: 1,
  date: "2026-04-13",
  note: null,
  skipped: false,
  components: [
    { id: 1, plate_id: 101, food_id: 1, portions: 1, sort_order: 0 },
  ],
  created_at: "2026-04-13T10:00:00Z",
}

export const mockPlateW16b: Plate = {
  id: 102,
  week_id: 1,
  day: 1,
  slot_id: 1,
  date: "2026-04-14",
  note: null,
  skipped: false,
  components: [
    { id: 2, plate_id: 102, food_id: 2, portions: 1, sort_order: 0 },
  ],
  created_at: "2026-04-14T10:00:00Z",
}

export const mockPlateW17: Plate = {
  id: 103,
  week_id: 2,
  day: 0,
  slot_id: 1,
  date: "2026-04-20",
  note: null,
  skipped: false,
  components: [
    { id: 3, plate_id: 103, food_id: 3, portions: 1, sort_order: 0 },
  ],
  created_at: "2026-04-20T10:00:00Z",
}
