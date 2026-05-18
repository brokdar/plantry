import { z } from "zod"

export const presetNameSchema = z.string().trim().min(1)
export const presetTagSchema = z.string().trim().min(1).max(40)

/** Save-as-preset dialog form. */
export const savePresetFormSchema = z.object({
  name: presetNameSchema,
  tags: z.array(presetTagSchema),
})

export type SavePresetFormValues = z.infer<typeof savePresetFormSchema>

/** Component shape inside the editor (kind-aware). */
export const presetComponentSchema = z
  .object({
    food_id: z.number().int().positive(),
    portions: z.number().int().positive().optional().nullable(),
    amount: z.number().positive().optional().nullable(),
    unit: z.string().trim().min(1).optional().nullable(),
    note: z.string().trim().max(500).optional().nullable(),
  })
  .refine(
    (v) => {
      const hasPortions = v.portions != null
      const hasLeaf = v.amount != null || (v.unit ?? "").length > 0
      return (hasPortions && !hasLeaf) || (!hasPortions && hasLeaf)
    },
    {
      message: "Must specify exactly one of portions or (amount + unit)",
      path: ["portions"],
    }
  )

export const presetPlateSchema = z.object({
  slot_id: z.number().int().positive(),
  components: z.array(presetComponentSchema).min(1),
})

export const presetEditorSchema = z.object({
  name: presetNameSchema,
  tags: z.array(presetTagSchema),
  plates: z.array(presetPlateSchema).min(1),
})

export type PresetEditorValues = z.infer<typeof presetEditorSchema>
