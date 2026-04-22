import { z } from "zod"

export const plateSchema = z.object({
  day: z.number().int().min(0).max(6),
  slot_id: z.number().int().positive(),
  note: z.string().nullable().optional(),
})

export type PlateFormValues = z.infer<typeof plateSchema>

export const plateComponentSchema = z.object({
  component_id: z.number().int().positive(),
  portions: z.number().positive(),
})

export type PlateComponentFormValues = z.infer<typeof plateComponentSchema>
