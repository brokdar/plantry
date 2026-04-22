import { z } from "zod"

export const profileSchema = z.object({
  kcal_target: z
    .number()
    .positive("Calorie target must be positive")
    .nullable()
    .optional(),
  protein_pct: z.number().min(0).nullable().optional(),
  fat_pct: z.number().min(0).nullable().optional(),
  carbs_pct: z.number().min(0).nullable().optional(),
  dietary_restrictions: z.array(z.string()).optional(),
  system_prompt: z.string().nullable().optional(),
  locale: z.string().min(1),
})

export type ProfileFormValues = z.infer<typeof profileSchema>
