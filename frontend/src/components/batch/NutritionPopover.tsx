import { TriangleAlert } from "lucide-react"
import { useId, type ReactElement } from "react"
import { useTranslation } from "react-i18next"

import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import type { PlanDay, PlanTargets } from "@/lib/api/plan"
import { formatDate, formatNumber } from "@/lib/plan/format"

interface NutritionPopoverProps {
  day: PlanDay
  targets: PlanTargets | null
  /** The trigger; it must accept a ref and props (a button). */
  children: ReactElement
}

/**
 * One day's per-eater kcal, protein, fat and carbs against the targets
 * (UX §5.5), opened from `children`. An incomplete day says so and names
 * the foods without nutrition data.
 */
export function NutritionPopover({
  day,
  targets,
  children,
}: NutritionPopoverProps) {
  const { t, i18n } = useTranslation()
  const titleId = useId()
  const rows = [
    {
      key: "kcal",
      value: day.macros.kcal,
      target: targets?.kcal,
      unit: "kcal",
    },
    {
      key: "protein",
      value: day.macros.protein,
      target: targets?.protein_g,
      unit: "g",
    },
    { key: "fat", value: day.macros.fat, target: targets?.fat_g, unit: "g" },
    {
      key: "carbs",
      value: day.macros.carbs,
      target: targets?.carbs_g,
      unit: "g",
    },
  ] as const

  return (
    <Popover>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent
        aria-labelledby={titleId}
        className="w-64 gap-3 bg-panel p-3 text-ink shadow-sheet ring-line"
      >
        <div>
          <p id={titleId} className="text-small font-semibold">
            {formatDate(day.date, i18n, {
              weekday: "long",
              month: "long",
              day: "numeric",
            })}
          </p>
          <p className="text-caption text-ink-muted">
            {t("day_meter.per_person")}
          </p>
        </div>
        <dl className="flex flex-col gap-1 text-small">
          {rows.map((row) => {
            const value = formatNumber(row.value, i18n)
            const unit = t(`units.${row.unit}`)
            return (
              <div key={row.key} className="flex justify-between gap-3">
                <dt className="text-ink-muted">{t(`day_meter.${row.key}`)}</dt>
                <dd className="tabular-nums">
                  {row.target
                    ? t("day_meter.amount_of", {
                        value,
                        target: formatNumber(row.target, i18n),
                        unit,
                      })
                    : t("day_meter.amount", { value, unit })}
                </dd>
              </div>
            )
          })}
        </dl>
        {!day.complete && (
          <p className="flex gap-1.5 text-small">
            <TriangleAlert
              className="mt-0.5 size-4 shrink-0 text-warning"
              strokeWidth={1.5}
              aria-hidden="true"
            />
            <span>
              <span className="font-semibold">{t("day_meter.incomplete")}</span>
              {day.missing.length > 0 && (
                <span className="block text-ink-muted">
                  {t("day_meter.missing", { names: day.missing.join(", ") })}
                </span>
              )}
            </span>
          </p>
        )}
      </PopoverContent>
    </Popover>
  )
}
