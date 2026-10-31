import { useTranslation } from "react-i18next"

import { BatchBadge } from "@/components/batch/BatchBadge"
import type { TodayRow } from "@/lib/plan/today"

/** One meal slot of the day (UX §5.2): the slot in Newsreader italic, then
 *  the plate with its batch badge, the routine, or No plate. */
export function MealRow({ row }: { row: TodayRow }) {
  const { cell } = row
  return (
    <li
      className="grid grid-cols-[6rem_minmax(0,1fr)] items-baseline gap-3 border-b border-line py-2.5 last:border-b-0"
      data-testid={`today-meal-${cell.slotId}`}
    >
      <span className="font-heading text-small text-ink-muted italic">
        {row.slotName}
      </span>
      <MealContent row={row} />
    </li>
  )
}

function MealContent({ row: { cell } }: { row: TodayRow }) {
  const { t } = useTranslation()
  if (cell.plate) {
    return (
      <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
        <span className="text-body text-ink">
          {cell.mainName ?? t("plan.cell.empty_plate")}
        </span>
        {cell.sideText && (
          <span className="text-small text-ink-muted">{cell.sideText}</span>
        )}
        {cell.badge && <BatchBadge {...cell.badge} />}
      </span>
    )
  }
  if (cell.mode === "none") {
    return (
      <span className="text-body text-ink-muted italic">
        {t("plan.cell.no_plate")}
        {cell.note && ` · ${cell.note}`}
      </span>
    )
  }
  if (cell.mode === "routine") {
    return (
      <span className="text-body text-ink">
        {cell.presetName
          ? t("plan.cell.routine_named", { name: cell.presetName })
          : t("plan.cell.routine")}
      </span>
    )
  }
  return (
    <span className="text-body text-ink-muted">{t("today.row.nothing")}</span>
  )
}
