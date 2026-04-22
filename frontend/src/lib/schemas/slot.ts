import { z } from "zod"

export const slotSchema = z.object({
  name_key: z.string().min(1, "name_key required"),
  icon: z.string().min(1, "icon required"),
  sort_order: z.number().int(),
  active: z.boolean(),
})

export type SlotFormValues = z.infer<typeof slotSchema>
