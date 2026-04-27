import { z } from "zod"

export const plateSchema = z.object({
  slot_id: z.number().int().positive(),
  note: z.string().nullable().optional(),
})

export type PlateFormValues = z.infer<typeof plateSchema>

export const plateComponentSchema = z.object({
  food_id: z.number().int().positive(),
  portions: z.number().positive(),
})

export type PlateComponentFormValues = z.infer<typeof plateComponentSchema>
