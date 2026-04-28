import { Trash2 } from "lucide-react"
import { useTranslation } from "react-i18next"

import type { MacrosResponse } from "@/lib/api/plates"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"
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
  const { t, i18n } = useTranslation()
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
        "group relative flex flex-col items-start gap-1.5 border-b border-outline-variant/50 px-2.5 py-2 pb-2.5",
        today &&
          "after:absolute after:inset-x-2.5 after:-bottom-px after:h-0.5 after:rounded-sm after:bg-primary"
      )}
      data-testid={`day-header-${idx}`}
      data-today={today ? "true" : undefined}
    >
      {/* Row 1: weekday + date */}
      <div className="flex items-baseline gap-1.5">
        <span
          className={cn(
            "font-heading text-[13px] font-bold tracking-widest uppercase",
            today ? "text-primary" : "text-on-surface"
          )}
        >
          {t(dayKey)}
        </span>
        <span className="text-[11px] text-on-surface-variant tabular-nums">
          {new Intl.DateTimeFormat(i18n.language, {
            month: "short",
            day: "numeric",
          }).format(date)}
        </span>
      </div>

      {/* Row 2: kcal + macro bar */}
      <div
        className="flex w-full items-center gap-2"
        data-testid="day-header-kcal"
      >
        <span
          className={cn(
            "font-heading text-[12px] font-bold tracking-tight tabular-nums",
            kcal > 0 ? "text-on-surface" : "text-on-surface-variant/60"
          )}
        >
          {kcal.toLocaleString()}
          <span className="ml-0.5 text-[9.5px] font-normal text-on-surface-variant">
            kcal
          </span>
        </span>
        {total > 0 ? (
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <div className="flex h-1 flex-1 cursor-default overflow-hidden rounded-full bg-surface-container">
                  <span
                    className="h-full bg-macro-protein"
                    style={{ width: `${pPct}%` }}
                  />
                  <span
                    className="h-full bg-macro-carbs"
                    style={{ width: `${cPct}%` }}
                  />
                  <span
                    className="h-full bg-macro-fat"
                    style={{ width: `${fPct}%` }}
                  />
                </div>
              </TooltipTrigger>
              <TooltipContent
                side="bottom"
                className="grid grid-cols-[8px_1fr_auto] items-center gap-x-1.5 gap-y-1"
              >
                <span className="size-2 rounded-full bg-macro-protein" />
                <span>{t("macro.protein")}</span>
                <span className="tabular-nums">{Math.round(pPct)}%</span>

                <span className="size-2 rounded-full bg-macro-carbs" />
                <span>{t("macro.carbs")}</span>
                <span className="tabular-nums">{Math.round(cPct)}%</span>

                <span className="size-2 rounded-full bg-macro-fat" />
                <span>{t("macro.fat")}</span>
                <span className="tabular-nums">{Math.round(fPct)}%</span>
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        ) : (
          <div className="h-1 flex-1 rounded-full bg-surface-container/60" />
        )}
      </div>

      {onClearDay && hasPlates && (
        <button
          type="button"
          onClick={onClearDay}
          aria-label={t("planner.clear_day")}
          data-testid={`clear-day-${idx}`}
          className="absolute top-1.5 right-1.5 grid size-5 place-items-center rounded text-on-surface-variant/40 opacity-0 transition-[opacity,color] group-hover:opacity-100 hover:text-destructive"
        >
          <Trash2 className="h-3 w-3" />
        </button>
      )}
    </div>
  )
}
