import { z } from "zod/v4"

export const COMPONENT_ROLES = [
  "main",
  "side_starch",
  "side_veg",
  "side_protein",
  "sauce",
  "drink",
  "dessert",
  "standalone",
] as const

export const componentIngredientSchema = z.object({
  ingredient_id: z.coerce.number().positive(),
  ingredient_name: z.string().optional(),
  amount: z.coerce.number().positive(),
  unit: z.string().min(1),
  grams: z.coerce.number().min(0).default(0),
  sort_order: z.coerce.number().min(0).default(0),
  // Per-100g macros stored client-side for live nutrition preview
  kcal_100g: z.coerce.number().min(0).default(0).optional(),
  protein_100g: z.coerce.number().min(0).default(0).optional(),
  fat_100g: z.coerce.number().min(0).default(0).optional(),
  carbs_100g: z.coerce.number().min(0).default(0).optional(),
  fiber_100g: z.coerce.number().min(0).default(0).optional(),
  sodium_100g: z.coerce.number().min(0).default(0).optional(),
})

export const instructionSchema = z.object({
  step_number: z.coerce.number().positive(),
  text: z.string().min(1),
})

export const componentSchema = z.object({
  name: z.string().min(1),
  role: z.enum(COMPONENT_ROLES),
  reference_portions: z.coerce.number().positive().default(1),
  prep_minutes: z.coerce.number().min(0).default(0),
  cook_minutes: z.coerce.number().min(0).default(0),
  notes: z.string().nullable().optional(),
  ingredients: z.array(componentIngredientSchema).min(1).default([]),
  instructions: z.array(instructionSchema).min(1).default([]),
  tags: z.array(z.string().min(1)).default([]),
})

export type ComponentFormValues = z.infer<typeof componentSchema>
