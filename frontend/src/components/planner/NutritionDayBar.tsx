import { useTranslation } from "react-i18next"

import { MacroDistributionBar } from "@/components/editorial/macros"
import type { MacrosResponse } from "@/lib/api/plates"

const DAY_LABELS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]

export interface NutritionTargets {
  kcal: number
  proteinG: number
  fatG: number
  carbsG: number
}

interface NutritionDayBarProps {
  day: number
  macros: MacrosResponse
  targets?: NutritionTargets
}

export function NutritionDayBar({
  day,
  macros,
  targets,
}: NutritionDayBarProps) {
  const { t } = useTranslation()
  const hasTargets = !!targets && targets.kcal > 0

  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span className="font-medium text-foreground">{DAY_LABELS[day]}</span>
        <span>
          {Math.round(macros.kcal)} {t("macro.kcal")}
          {hasTargets && (
            <span className="ml-1 text-muted-foreground/70">
              / {targets!.kcal}
            </span>
          )}
        </span>
      </div>
      <MacroDistributionBar
        thickness="md"
        track="muted"
        mode={hasTargets ? "targets" : "kcal"}
        values={{
          protein: macros.protein,
          carbs: macros.carbs,
          fat: macros.fat,
        }}
        targets={
          hasTargets
            ? {
                protein: targets!.proteinG,
                carbs: targets!.carbsG,
                fat: targets!.fatG,
              }
            : undefined
        }
        label={`${DAY_LABELS[day]}: ${Math.round(macros.kcal)} ${t("macro.kcal")}`}
      />
      <div className="flex gap-3 text-xs text-muted-foreground">
        <span>
          {t("macro.protein_abbr")} {macros.protein.toFixed(0)}g
        </span>
        <span>
          {t("macro.fat_abbr")} {macros.fat.toFixed(0)}g
        </span>
        <span>
          {t("macro.carbs_abbr")} {macros.carbs.toFixed(0)}g
        </span>
      </div>
    </div>
  )
}
