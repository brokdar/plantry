import { z } from "zod"

export const slotSchema = z.object({
  name_key: z.string().refine((v) => v.length > 0, {
    message: "name_key required",
  }),
  icon: z.string().min(1),
  sort_order: z.number().int(),
  active: z.boolean(),
})

export type SlotFormValues = z.infer<typeof slotSchema>
