import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { BatchBadge } from "@/components/batch/BatchBadge"
import type { Board } from "@/lib/plan/board"
import { cn } from "@/lib/utils"

interface WeekStripProps {
  board: Board
  windowStart: string
  /** The day Today shows; its row is marked. */
  date: string
}

/**
 * "This week" (UX §5.2, tablet and desktop): the window's days, read-only,
 * with each plate's name and batch badge. A day opens the plan board at
 * that day.
 */
export function WeekStrip({ board, windowStart, date }: WeekStripProps) {
  const { t } = useTranslation()
  return (
    <nav aria-labelledby="today-week" className="flex flex-col gap-2">
      <h2
        id="today-week"
        className="font-sans text-small font-semibold text-ink"
      >
        {t("today.week.title")}
      </h2>
      <ol className="flex flex-col border-t border-line">
        {board.days.map((day) => {
          const plated = day.cells.filter((c) => c.plate)
          const current = day.date === date
          return (
            <li key={day.date} className="border-b border-line">
              <Link
                to="/plan"
                search={{ start: windowStart, day: day.date }}
                aria-current={current ? "date" : undefined}
                title={t("today.week.open", { day: day.weekday })}
                className="grid min-h-11 grid-cols-[2.75rem_minmax(0,1fr)] gap-3 rounded-cell px-2 py-2 transition-colors duration-150 ease-out hover:bg-panel-sunken"
                data-testid={`week-strip-${day.date}`}
              >
                <span
                  className={cn(
                    "flex flex-col leading-tight",
                    current ? "text-accent" : "text-ink"
                  )}
                >
                  <span className="text-caption">{day.weekdayShort}</span>
                  <span className="text-body font-semibold tabular-nums">
                    {day.dayNumber}
                  </span>
                </span>
                <span className="flex min-w-0 flex-col justify-center gap-1">
                  {plated.length === 0 ? (
                    <span className="text-small text-ink-muted">
                      {t("today.week.empty")}
                    </span>
                  ) : (
                    plated.map((cell) => (
                      <span
                        key={cell.key}
                        className="flex min-w-0 items-center gap-2"
                      >
                        {cell.badge && <BatchBadge {...cell.badge} />}
                        <span className="truncate text-small text-ink">
                          {cell.mainName ?? t("plan.cell.empty_plate")}
                        </span>
                      </span>
                    ))
                  )}
                </span>
              </Link>
            </li>
          )
        })}
      </ol>
    </nav>
  )
}
