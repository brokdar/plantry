import { useMemo } from "react"
import { useTranslation } from "react-i18next"
import { fromIngredients, type IngredientInput } from "@/lib/domain/nutrition"

interface NutritionPreviewProps {
  ingredients: {
    grams: number
    kcal_100g?: number
    protein_100g?: number
    fat_100g?: number
    carbs_100g?: number
    fiber_100g?: number
    sodium_100g?: number
  }[]
  referencePortions: number
}

export function NutritionPreview({
  ingredients,
  referencePortions,
}: NutritionPreviewProps) {
  const { t } = useTranslation()

  const macros = useMemo(() => {
    const inputs: IngredientInput[] = ingredients
      .filter((i) => i.grams > 0)
      .map((i) => ({
        per_100g: {
          kcal: i.kcal_100g ?? 0,
          protein: i.protein_100g ?? 0,
          fat: i.fat_100g ?? 0,
          carbs: i.carbs_100g ?? 0,
          fiber: i.fiber_100g ?? 0,
          sodium: i.sodium_100g ?? 0,
        },
        grams: i.grams,
      }))
    const total = fromIngredients(inputs)
    const portions = referencePortions > 0 ? referencePortions : 1
    return {
      kcal: total.kcal / portions,
      protein: total.protein / portions,
      fat: total.fat / portions,
      carbs: total.carbs / portions,
      fiber: total.fiber / portions,
      sodium: total.sodium / portions,
    }
  }, [ingredients, referencePortions])

  const fields = [
    { label: t("ingredient.kcal"), value: macros.kcal },
    { label: t("ingredient.protein"), value: macros.protein },
    { label: t("ingredient.fat"), value: macros.fat },
    { label: t("ingredient.carbs"), value: macros.carbs },
    { label: t("ingredient.fiber"), value: macros.fiber },
    { label: t("ingredient.sodium"), value: macros.sodium },
  ]

  return (
    <div className="space-y-2">
      <h3 className="text-sm font-medium">{t("component.nutrition")}</h3>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
        {fields.map((f) => (
          <div
            key={f.label}
            className="rounded-md border border-border bg-muted/50 px-3 py-2"
          >
            <p className="text-xs text-muted-foreground">{f.label}</p>
            <p className="text-sm font-medium">{f.value.toFixed(1)}</p>
          </div>
        ))}
      </div>
    </div>
  )
}
