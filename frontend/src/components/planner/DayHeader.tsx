import { format } from "date-fns"
import { Trash2 } from "lucide-react"
import { useTranslation } from "react-i18next"

import type { MacrosResponse } from "@/lib/api/weeks"
import { cn } from "@/lib/utils"

interface DayHeaderProps {
  date: Date
  dayKey: string
  idx: number
  today?: boolean
  macros?: MacrosResponse
  onClearDay?: () => void
  hasPlates?: boolean
}

export function DayHeader({
  date,
  dayKey,
  idx,
  today,
  macros,
  onClearDay,
  hasPlates,
}: DayHeaderProps) {
  const { t } = useTranslation()
  // Always render kcal + macro strip on every day header so the row keeps a
  // uniform vertical rhythm — empty days just read "0 kcal" with a faded bar.
  const kcal = macros ? Math.round(macros.kcal) : 0
  const total =
    macros && macros.protein + macros.carbs + macros.fat > 0
      ? macros.protein + macros.carbs + macros.fat
      : 0
  const pPct = total > 0 ? (macros!.protein / total) * 100 : 0
  const cPct = total > 0 ? (macros!.carbs / total) * 100 : 0
  const fPct = total > 0 ? (macros!.fat / total) * 100 : 0

  return (
    <div
      className={cn(
        "group relative flex flex-col items-start gap-2 border-b border-outline-variant/50 px-2.5 py-3 pb-3.5",
        today &&
          "after:absolute after:inset-x-2.5 after:-bottom-px after:h-0.5 after:rounded-sm after:bg-primary"
      )}
      data-testid={`day-header-${idx}`}
    >
      <span
        className={cn(
          "font-heading text-[13px] font-bold tracking-widest uppercase",
          today ? "text-primary" : "text-on-surface"
        )}
      >
        {t(dayKey)}
      </span>
      <span className="text-[12px] text-on-surface-variant tabular-nums">
        {format(date, "MMM d")}
        {today && <span className="ml-2">· {t("planner.today")}</span>}
      </span>
      <span className="flex items-baseline gap-1" data-testid="day-header-kcal">
        <span
          className={cn(
            "font-heading text-[14px] font-bold tracking-tight",
            kcal > 0 ? "text-on-surface" : "text-on-surface-variant/60"
          )}
        >
          {kcal.toLocaleString()}
        </span>
        <span className="text-[10.5px] text-on-surface-variant">kcal</span>
      </span>
      {total > 0 ? (
        <div className="mt-0.5 flex h-1 w-full overflow-hidden rounded-full bg-surface-container">
          <span
            className="h-full bg-macro-protein"
            style={{ width: `${pPct}%` }}
          />
          <span
            className="h-full bg-macro-carbs"
            style={{ width: `${cPct}%` }}
          />
          <span className="h-full bg-macro-fat" style={{ width: `${fPct}%` }} />
        </div>
      ) : (
        <div className="mt-0.5 h-1 w-full rounded-full bg-surface-container/60" />
      )}
      {onClearDay && hasPlates && (
        <button
          type="button"
          onClick={onClearDay}
          aria-label={t("planner.clear_day")}
          data-testid={`clear-day-${idx}`}
          className="absolute top-2 right-1.5 grid size-5 place-items-center rounded text-on-surface-variant/40 opacity-0 transition-[opacity,color] group-hover:opacity-100 hover:text-destructive"
        >
          <Trash2 className="h-3 w-3" />
        </button>
      )}
    </div>
  )
}
