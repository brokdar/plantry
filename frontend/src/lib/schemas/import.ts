import { z } from "zod/v4"
import { FOOD_ROLES } from "./food"

export const importStep1Schema = z
  .object({
    url: z.string().trim(),
    html: z.string().trim().optional(),
  })
  .refine((v) => v.url !== "" || (v.html && v.html !== ""), {
    message: "import.step1.url_or_html_required",
  })

export type ImportStep1Values = z.infer<typeof importStep1Schema>

export const importFinalizeSchema = z.object({
  name: z.string().min(1),
  role: z.enum(FOOD_ROLES),
  reference_portions: z.coerce.number().positive(),
  prep_minutes: z.coerce.number().min(0).nullable(),
  cook_minutes: z.coerce.number().min(0).nullable(),
})

export type ImportFinalizeValues = z.infer<typeof importFinalizeSchema>
