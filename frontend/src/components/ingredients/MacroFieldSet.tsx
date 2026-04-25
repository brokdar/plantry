import { type Control, type FieldValues, type Path } from "react-hook-form"
import { useTranslation } from "react-i18next"
import {
  FormField,
  FormItem,
  FormLabel,
  FormControl,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import type { LeafFoodFormValues } from "@/lib/schemas/food"

interface MacroFieldSetProps<T extends FieldValues = LeafFoodFormValues> {
  control: Control<T>
  disabled?: boolean
}

// Sodium is stored server-side as grams per 100 g but the UI presents mg,
// which is the unit on every food label. displayScale/storeScale converts
// between the two at the form boundary so users type what they see.
const MACRO_FIELDS = [
  { name: "kcal_100g", labelKey: "ingredient.kcal" },
  { name: "protein_100g", labelKey: "ingredient.protein" },
  { name: "fat_100g", labelKey: "ingredient.fat" },
  { name: "carbs_100g", labelKey: "ingredient.carbs" },
  { name: "fiber_100g", labelKey: "ingredient.fiber" },
  { name: "sodium_100g", labelKey: "ingredient.sodium", displayScale: 1000 },
] as const

export function MacroFieldSet({ control, disabled }: MacroFieldSetProps) {
  const { t } = useTranslation()

  return (
    <div className="grid grid-cols-2 gap-4 md:grid-cols-3">
      {MACRO_FIELDS.map((field) => {
        const scale = "displayScale" in field ? field.displayScale : 1
        return (
          <FormField
            key={field.name}
            control={control}
            name={field.name as Path<LeafFoodFormValues>}
            render={({ field: fieldProps }) => {
              const stored = (fieldProps.value as number | null) ?? null
              const displayValue =
                stored == null
                  ? ""
                  : String(Math.round(stored * scale * 100) / 100)
              return (
                <FormItem>
                  <FormLabel>{t(field.labelKey)}</FormLabel>
                  <FormControl>
                    <Input
                      type="number"
                      step="0.1"
                      min="0"
                      disabled={disabled}
                      name={fieldProps.name}
                      ref={fieldProps.ref}
                      onBlur={fieldProps.onBlur}
                      value={displayValue}
                      onChange={(e) => {
                        const raw = e.target.value
                        if (raw === "") {
                          fieldProps.onChange(0)
                          return
                        }
                        const parsed = Number(raw)
                        if (Number.isNaN(parsed)) {
                          fieldProps.onChange(stored)
                          return
                        }
                        fieldProps.onChange(parsed / scale)
                      }}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )
            }}
          />
        )
      })}
    </div>
  )
}
