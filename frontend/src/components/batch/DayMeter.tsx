import { useTranslation } from "react-i18next"

import type { PlanDay, PlanTargets } from "@/lib/api/plan"
import { formatDate, formatNumber } from "@/lib/plan/format"
import { cn } from "@/lib/utils"

import { NutritionPopover } from "./NutritionPopover"

/** On target between 70 % and 115 % of the kcal target (UX §5.5). */
const ON_TARGET_MIN = 0.7
const ON_TARGET_MAX = 1.15
/** The track runs to 125 % of the target, so the target tick sits at 80 %
 *  and an overshoot reads by length, not only by colour. */
const TRACK_MAX = 1.25

const FILL = {
  success: "bg-success",
  warning: "bg-warning",
  neutral: "bg-ink-muted",
} as const

interface DayMeterProps {
  day: PlanDay
  targets: PlanTargets | null
  className?: string
}

/**
 * A thin bar of the day's per-eater kcal against the target: success colour
 * on target, warning colour outside, neutral without a target. The track
 * runs to 125 % of the target with a tick at the target. An incomplete day
 * adds a warning dot and says so in the value text. The whole meter opens
 * the `NutritionPopover`.
 */
export function DayMeter({ day, targets, className }: DayMeterProps) {
  const { t, i18n } = useTranslation()
  const kcal = day.macros.kcal
  const target = targets?.kcal || null
  const ratio = target ? kcal / target : null
  const status =
    ratio === null
      ? "neutral"
      : ratio >= ON_TARGET_MIN && ratio <= ON_TARGET_MAX
        ? "success"
        : "warning"
  const unit = t("units.kcal")
  const amountText = target
    ? t("day_meter.value", {
        kcal: formatNumber(kcal, i18n),
        target: formatNumber(target, i18n),
        unit,
      })
    : t("day_meter.value_no_target", { kcal: formatNumber(kcal, i18n), unit })
  const valueText = day.complete
    ? amountText
    : t("day_meter.value_incomplete", { value: amountText })

  return (
    <div
      className={cn("relative flex min-h-11 items-center gap-1.5", className)}
    >
      <div
        role="meter"
        aria-label={t("day_meter.name")}
        aria-valuemin={0}
        aria-valuemax={Math.round(target ?? Math.max(kcal, 1))}
        aria-valuenow={Math.round(kcal)}
        aria-valuetext={valueText}
        data-status={status}
        className="relative h-1 flex-1 rounded-full bg-line"
      >
        <div
          className={cn("h-full rounded-full", FILL[status])}
          style={{
            width: `${Math.min(ratio === null ? 1 : ratio / TRACK_MAX, 1) * 100}%`,
          }}
        />
        {target && (
          <span
            data-testid="day-meter-target"
            className="absolute -inset-y-0.5 w-px bg-ink"
            style={{ left: `${100 / TRACK_MAX}%` }}
            aria-hidden="true"
          />
        )}
      </div>
      {!day.complete && (
        <span
          data-testid="day-meter-incomplete"
          className="size-1.5 shrink-0 rounded-full bg-warning"
          aria-hidden="true"
        />
      )}
      {/* Stretched over the meter so the whole row is the 44px target; a
          sibling, not a parent, so the meter stays in the a11y tree. */}
      <NutritionPopover day={day} targets={targets}>
        <button
          type="button"
          aria-label={t("day_meter.open", {
            day: formatDate(day.date, i18n, {
              weekday: "long",
              month: "long",
              day: "numeric",
            }),
          })}
          className="absolute inset-0 rounded-cell"
        />
      </NutritionPopover>
    </div>
  )
}
