import { z } from "zod/v4"

// Optional nutrient: coerces "" → null so empty inputs serialize correctly.
const optionalNutrient = z.preprocess(
  (v) => (v === "" || v === undefined ? null : v),
  z.coerce.number().min(0).nullable()
)

export const ingredientSchema = z.object({
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

export type IngredientFormValues = z.infer<typeof ingredientSchema>
