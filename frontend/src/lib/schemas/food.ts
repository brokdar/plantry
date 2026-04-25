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
  kcal_100g: z.coerce.number().min(0).default(0),
  protein_100g: z.coerce.number().min(0).default(0),
  fat_100g: z.coerce.number().min(0).default(0),
  carbs_100g: z.coerce.number().min(0).default(0),
  fiber_100g: z.coerce.number().min(0).default(0),
  sodium_100g: z.coerce.number().min(0).default(0),

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

export const composedFoodSchema = z.object({
  kind: z.literal("composed"),
  name: z.string().min(1),
  role: z.enum(FOOD_ROLES).optional(),
  reference_portions: z.coerce.number().positive().default(1),
  prep_minutes: z.coerce.number().min(0).default(0),
  cook_minutes: z.coerce.number().min(0).default(0),
  notes: z.string().nullable().optional(),
  children: z.array(composedChildSchema).default([]),
  instructions: z.array(composedInstructionSchema).default([]),
  tags: z.array(z.string().min(1)).default([]),
})

export const foodSchema = z.discriminatedUnion("kind", [
  leafFoodSchema,
  composedFoodSchema,
])

export type LeafFoodFormValues = z.infer<typeof leafFoodSchema>
export type ComposedFoodFormValues = z.infer<typeof composedFoodSchema>
export type ComposedChildFormValues = z.infer<typeof composedChildSchema>
export type FoodFormValues = z.infer<typeof foodSchema>
