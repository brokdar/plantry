import { z } from "zod/v4"

export const FOOD_ROLES = [
  "main",
  "side_starch",
  "side_veg",
  "side_protein",
  "sauce",
  "drink",
  "dessert",
  "standalone",
] as const

// Optional nutrient: coerces "" → null so empty inputs serialize correctly.
const optionalNutrient = z.preprocess(
  (v) => (v === "" || v === undefined ? null : v),
  z.coerce.number().min(0).nullable()
)

const leafBase = z.object({
  kind: z.literal("leaf"),
  name: z.string().min(1),
  source: z.string().optional().default("manual"),
  barcode: z.string().nullable().optional(),
  off_id: z.string().nullable().optional(),
  fdc_id: z.string().nullable().optional(),
  // Empty means unknown, not zero (Phase 0 C2 nutrition completeness).
  kcal_100g: optionalNutrient.default(null),
  protein_100g: optionalNutrient.default(null),
  fat_100g: optionalNutrient.default(null),
  carbs_100g: optionalNutrient.default(null),
  fiber_100g: optionalNutrient.default(null),
  sodium_100g: optionalNutrient.default(null),

  saturated_fat_100g: optionalNutrient.default(null),
  trans_fat_100g: optionalNutrient.default(null),
  cholesterol_100g: optionalNutrient.default(null),
  sugar_100g: optionalNutrient.default(null),
  potassium_100g: optionalNutrient.default(null),
  calcium_100g: optionalNutrient.default(null),
  iron_100g: optionalNutrient.default(null),
  magnesium_100g: optionalNutrient.default(null),
  phosphorus_100g: optionalNutrient.default(null),
  zinc_100g: optionalNutrient.default(null),
  vitamin_a_100g: optionalNutrient.default(null),
  vitamin_c_100g: optionalNutrient.default(null),
  vitamin_d_100g: optionalNutrient.default(null),
  vitamin_b12_100g: optionalNutrient.default(null),
  vitamin_b6_100g: optionalNutrient.default(null),
  folate_100g: optionalNutrient.default(null),
})

export const leafFoodSchema = leafBase

export const composedChildSchema = z.object({
  child_id: z.coerce.number().positive(),
  child_name: z.string().optional(),
  child_kind: z.enum(["leaf", "composed"]).optional(),
  amount: z.coerce.number().positive(),
  unit: z.string().min(1),
  grams: z.coerce.number().min(0).default(0),
  sort_order: z.coerce.number().min(0).default(0),
  // Per-100g macros stored client-side for live nutrition preview.
  kcal_100g: z.coerce.number().min(0).default(0).optional(),
  protein_100g: z.coerce.number().min(0).default(0).optional(),
  fat_100g: z.coerce.number().min(0).default(0).optional(),
  carbs_100g: z.coerce.number().min(0).default(0).optional(),
  fiber_100g: z.coerce.number().min(0).default(0).optional(),
  sodium_100g: z.coerce.number().min(0).default(0).optional(),
})

export const composedInstructionSchema = z.object({
  step_number: z.coerce.number().positive(),
  text: z.string().min(1),
})

const optionalMinutes = z.preprocess(
  (v) => (v === "" || v === undefined ? null : v),
  z.coerce.number().int().min(0).nullable()
)

export const composedFoodSchema = z.object({
  kind: z.literal("composed"),
  name: z.string().min(1),
  role: z.enum(FOOD_ROLES).optional(),
  reference_portions: z.coerce.number().positive().default(1),
  // Empty means unknown (null), not 0: 0 would read as a quick cook.
  prep_minutes: optionalMinutes,
  cook_minutes: optionalMinutes,
  // 1–7 days; empty means "use plan.default_keep_days" (D15).
  keep_days: z.preprocess(
    (v) => (v === "" || v === undefined ? null : v),
    z.coerce.number().int().min(1).max(7).nullable()
  ),
  freezable: z.boolean().nullable().default(null),
  notes: z.string().nullable().optional(),
  // "" → undefined so an empty field passes; the editor sends "" to clear.
  source_url: z.preprocess(
    (v) => (typeof v === "string" && v.trim() !== "" ? v.trim() : undefined),
    z.url({ protocol: /^https?$/ }).optional()
  ),
  description: z.string().nullable().optional(),
  children: z.array(composedChildSchema).default([]),
  instructions: z.array(composedInstructionSchema).default([]),
  tags: z.array(z.string().min(1)).default([]),
})

export type LeafFoodFormValues = z.infer<typeof leafFoodSchema>
export type ComposedFoodFormValues = z.infer<typeof composedFoodSchema>
