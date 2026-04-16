import { useTranslation } from "react-i18next"

import { Separator } from "@/components/ui/separator"
import { useWeekNutrition } from "@/lib/queries/weeks"

import { NutritionDayBar } from "./NutritionDayBar"

interface NutritionWeekSummaryProps {
  weekId: number
}

export function NutritionWeekSummary({ weekId }: NutritionWeekSummaryProps) {
  const { t } = useTranslation()
  const { data, isLoading } = useWeekNutrition(weekId)

  if (isLoading) {
    return (
      <p className="px-4 py-4 text-sm text-muted-foreground">
        {t("common.loading")}
      </p>
    )
  }

  if (!data) return null

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
              <NutritionDayBar day={d.day} macros={d.macros} />
            </li>
          ))}
        </ul>
      )}

      <Separator />

      <div className="space-y-1 px-4">
        <div className="flex items-center justify-between text-sm font-medium">
          <span>{t("nutrition.week_total")}</span>
          <span>{Math.round(data.week.kcal)} kcal</span>
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
