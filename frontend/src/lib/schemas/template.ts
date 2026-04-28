import { z } from "zod"

export const templateSchema = z.object({
  name: z.string().trim().min(1),
})

export type TemplateFormValues = z.infer<typeof templateSchema>
