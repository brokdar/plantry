import { type Control, type Path } from "react-hook-form"
import { useTranslation } from "react-i18next"

import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import { cn } from "@/lib/utils"

import type { IngredientFormValues } from "@/lib/schemas/ingredient"

type FieldDef = {
  name: keyof IngredientFormValues
  labelKey: string
  unit: string
}

interface ExtendedNutrientFieldSetProps {
  control: Control<IngredientFormValues>
  fields: FieldDef[]
  disabled?: boolean
}

/**
 * Tile grid for optional extended nutrients. Each tile is a compact surface
 * panel with a tracked uppercase label, a prominent numeric value, and a
 * unit glyph on the right — matching the NutrientInput aesthetic used in the
 * old Plantry editor. An empty string is stored as null so "no upstream data"
 * stays distinct from "0".
 */
export function ExtendedNutrientFieldSet({
  control,
  fields,
  disabled,
}: ExtendedNutrientFieldSetProps) {
  const { t } = useTranslation()
  return (
    <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 md:grid-cols-4">
      {fields.map((field) => (
        <FormField
          key={String(field.name)}
          control={control}
          name={field.name as Path<IngredientFormValues>}
          render={({ field: fieldProps, fieldState }) => {
            const stored = fieldProps.value as number | null | undefined
            const displayValue = stored == null ? "" : String(stored)
            const filled = stored != null
            return (
              <FormItem className="space-y-0">
                <div
                  className={cn(
                    "group relative rounded-xl bg-surface-container-low p-3 transition-colors",
                    "focus-within:bg-surface-container focus-within:ring-1 focus-within:ring-primary/30",
                    filled && "border-l-2 border-primary/40",
                    fieldState.invalid && "ring-1 ring-destructive/60"
                  )}
                >
                  <div className="mb-1 flex items-center justify-between gap-2">
                    <FormLabel className="text-xs font-normal text-on-surface-variant">
                      {t(field.labelKey)}
                    </FormLabel>
                    <span className="font-mono text-[10px] text-on-surface-variant/60">
                      {field.unit}
                    </span>
                  </div>
                  <FormControl>
                    <input
                      type="number"
                      step="0.1"
                      min="0"
                      inputMode="decimal"
                      disabled={disabled}
                      name={fieldProps.name}
                      ref={fieldProps.ref}
                      onBlur={fieldProps.onBlur}
                      value={displayValue}
                      placeholder="—"
                      onChange={(e) => {
                        const raw = e.target.value
                        if (raw === "") {
                          fieldProps.onChange(null)
                          return
                        }
                        const parsed = Number(raw)
                        if (Number.isNaN(parsed)) return
                        fieldProps.onChange(parsed)
                      }}
                      className={cn(
                        "w-full bg-transparent text-lg font-semibold text-on-surface tabular-nums",
                        "[appearance:textfield] focus:outline-none",
                        "[&::-webkit-inner-spin-button]:appearance-none",
                        "[&::-webkit-outer-spin-button]:appearance-none",
                        "placeholder:font-normal placeholder:text-on-surface-variant/40"
                      )}
                    />
                  </FormControl>
                  <FormMessage className="mt-1 text-[10px]" />
                </div>
              </FormItem>
            )
          }}
        />
      ))}
    </div>
  )
}

export const EXTENDED_MACRO_FIELDS: FieldDef[] = [
  {
    name: "saturated_fat_100g",
    labelKey: "nutrition.saturated_fat",
    unit: "g",
  },
  { name: "trans_fat_100g", labelKey: "nutrition.trans_fat", unit: "g" },
  { name: "cholesterol_100g", labelKey: "nutrition.cholesterol", unit: "mg" },
  { name: "sugar_100g", labelKey: "nutrition.sugar", unit: "g" },
]

export const MINERAL_FIELDS: FieldDef[] = [
  { name: "potassium_100g", labelKey: "nutrition.potassium", unit: "mg" },
  { name: "calcium_100g", labelKey: "nutrition.calcium", unit: "mg" },
  { name: "iron_100g", labelKey: "nutrition.iron", unit: "mg" },
  { name: "magnesium_100g", labelKey: "nutrition.magnesium", unit: "mg" },
  { name: "phosphorus_100g", labelKey: "nutrition.phosphorus", unit: "mg" },
  { name: "zinc_100g", labelKey: "nutrition.zinc", unit: "mg" },
]

export const VITAMIN_FIELDS: FieldDef[] = [
  { name: "vitamin_a_100g", labelKey: "nutrition.vitamin_a", unit: "µg" },
  { name: "vitamin_c_100g", labelKey: "nutrition.vitamin_c", unit: "mg" },
  { name: "vitamin_d_100g", labelKey: "nutrition.vitamin_d", unit: "µg" },
  { name: "vitamin_b12_100g", labelKey: "nutrition.vitamin_b12", unit: "µg" },
  { name: "vitamin_b6_100g", labelKey: "nutrition.vitamin_b6", unit: "mg" },
  { name: "folate_100g", labelKey: "nutrition.folate", unit: "µg" },
]
