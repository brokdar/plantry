import { z } from "zod/v4"

export const ingredientSchema = z.object({
  name: z.string().min(1),
  kcal_100g: z.coerce.number().min(0).default(0),
  protein_100g: z.coerce.number().min(0).default(0),
  fat_100g: z.coerce.number().min(0).default(0),
  carbs_100g: z.coerce.number().min(0).default(0),
  fiber_100g: z.coerce.number().min(0).default(0),
  sodium_100g: z.coerce.number().min(0).default(0),
})

export type IngredientFormValues = z.infer<typeof ingredientSchema>
