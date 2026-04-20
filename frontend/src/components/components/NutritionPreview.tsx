import { useMemo } from "react"
import { useTranslation } from "react-i18next"

import {
  MacroDistributionBar,
  MacroKcalHero,
  MacroTriad,
} from "@/components/editorial/macros"
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

  return (
    <div className="space-y-3">
      <h3 className="text-sm font-medium">{t("component.nutrition")}</h3>
      <MacroKcalHero kcal={macros.kcal} size="md" />
      <MacroDistributionBar
        thickness="md"
        values={{
          protein: macros.protein,
          carbs: macros.carbs,
          fat: macros.fat,
        }}
      />
      <MacroTriad
        size="sm"
        values={{
          protein: macros.protein,
          carbs: macros.carbs,
          fat: macros.fat,
        }}
      />
      <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs text-on-surface-variant">
        <div className="flex items-center justify-between">
          <dt>{t("macro.fiber")}</dt>
          <dd className="font-medium text-on-surface">
            {macros.fiber.toFixed(1)} g
          </dd>
        </div>
        <div className="flex items-center justify-between">
          <dt>{t("ingredient.sodium")}</dt>
          <dd className="font-medium text-on-surface">
            {(macros.sodium * 1000).toFixed(0)} mg
          </dd>
        </div>
      </dl>
    </div>
  )
}
