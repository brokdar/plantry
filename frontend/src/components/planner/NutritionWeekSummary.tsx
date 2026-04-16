import { useTranslation } from "react-i18next"

import { Separator } from "@/components/ui/separator"
import type { Profile } from "@/lib/api/profile"
import { useProfile } from "@/lib/queries/profile"
import { useWeekNutrition } from "@/lib/queries/weeks"

import { NutritionDayBar, type NutritionTargets } from "./NutritionDayBar"

interface NutritionWeekSummaryProps {
  weekId: number
}

function dailyTargets(profile: Profile): NutritionTargets | undefined {
  if (!profile.kcal_target) return undefined
  const kcal = profile.kcal_target
  return {
    kcal,
    proteinG: profile.protein_pct ? (kcal * profile.protein_pct) / 100 / 4 : 0,
    fatG: profile.fat_pct ? (kcal * profile.fat_pct) / 100 / 9 : 0,
    carbsG: profile.carbs_pct ? (kcal * profile.carbs_pct) / 100 / 4 : 0,
  }
}

export function NutritionWeekSummary({ weekId }: NutritionWeekSummaryProps) {
  const { t } = useTranslation()
  const { data, isLoading } = useWeekNutrition(weekId)
  const { data: profile } = useProfile()

  if (isLoading) {
    return (
      <p className="px-4 py-4 text-sm text-muted-foreground">
        {t("common.loading")}
      </p>
    )
  }

  if (!data) return null

  const targets = profile ? dailyTargets(profile) : undefined

  const weekAvgKcal =
    data.days.length > 0 ? Math.round(data.week.kcal / data.days.length) : 0

  return (
    <div className="flex flex-col gap-4 overflow-y-auto py-4">
      {data.days.length === 0 ? (
        <p className="px-4 text-sm text-muted-foreground">
          {t("nutrition.empty")}
        </p>
      ) : (
        <ul className="flex flex-col gap-4 px-4">
          {data.days.map((d) => (
            <li key={d.day}>
              <NutritionDayBar
                day={d.day}
                macros={d.macros}
                targets={targets}
              />
            </li>
          ))}
        </ul>
      )}

      <Separator />

      <div className="space-y-1 px-4">
        <div className="flex items-center justify-between text-sm font-medium">
          <span>{t("nutrition.week_total")}</span>
          <span>
            {Math.round(data.week.kcal)} kcal
            {targets && data.days.length > 0 && (
              <span className="ml-1 text-xs text-muted-foreground">
                (avg {weekAvgKcal} / {targets.kcal})
              </span>
            )}
          </span>
        </div>
        <div className="flex gap-4 text-xs text-muted-foreground">
          <span>P {data.week.protein.toFixed(0)}g</span>
          <span>F {data.week.fat.toFixed(0)}g</span>
          <span>C {data.week.carbs.toFixed(0)}g</span>
        </div>
      </div>
    </div>
  )
}
