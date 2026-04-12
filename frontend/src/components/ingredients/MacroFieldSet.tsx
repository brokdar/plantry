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
import type { IngredientFormValues } from "@/lib/schemas/ingredient"

interface MacroFieldSetProps<T extends FieldValues = IngredientFormValues> {
  control: Control<T>
  disabled?: boolean
}

const MACRO_FIELDS = [
  { name: "kcal_100g", labelKey: "ingredient.kcal" },
  { name: "protein_100g", labelKey: "ingredient.protein" },
  { name: "fat_100g", labelKey: "ingredient.fat" },
  { name: "carbs_100g", labelKey: "ingredient.carbs" },
  { name: "fiber_100g", labelKey: "ingredient.fiber" },
  { name: "sodium_100g", labelKey: "ingredient.sodium" },
] as const

export function MacroFieldSet({ control, disabled }: MacroFieldSetProps) {
  const { t } = useTranslation()

  return (
    <div className="grid grid-cols-2 gap-4 md:grid-cols-3">
      {MACRO_FIELDS.map((field) => (
        <FormField
          key={field.name}
          control={control}
          name={field.name as Path<IngredientFormValues>}
          render={({ field: fieldProps }) => (
            <FormItem>
              <FormLabel>{t(field.labelKey)}</FormLabel>
              <FormControl>
                <Input
                  type="number"
                  step="0.1"
                  min="0"
                  disabled={disabled}
                  {...fieldProps}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      ))}
    </div>
  )
}
