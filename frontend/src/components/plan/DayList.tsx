import { DayMeter } from "@/components/batch/DayMeter"
import { cn } from "@/lib/utils"

import type { BoardLayoutProps } from "./BoardGrid"
import { PlateCell } from "./PlateCell"

/**
 * The phone board (UX §5.5): one block per day with the day meter in its
 * header and the meals as rows. Rows that are routine every day show once,
 * above the days. Same view model as the grid; no drag and drop.
 */
export function DayList({
  board,
  targets,
  label,
  onOpenCell,
}: BoardLayoutProps) {
  const collapsed = board.rows.filter((r) => r.collapsed)
  const slotRows = board.rows.filter((r) => !r.collapsed)

  return (
    <section aria-label={label} className="flex flex-col gap-6">
      {collapsed.length > 0 && (
        <ul className="flex flex-col gap-1 text-small text-ink-muted">
          {collapsed.map((row) => (
            <li key={row.slotId}>{row.collapsed}</li>
          ))}
        </ul>
      )}
      {board.days.map((day) => {
        const headingId = `plan-day-${day.date}`
        return (
          <section
            key={day.date}
            aria-labelledby={headingId}
            className="flex flex-col gap-2"
          >
            <header className="flex items-center gap-3 border-b border-line pb-1">
              <h2
                id={headingId}
                className={cn(
                  "shrink-0 font-sans text-body font-semibold",
                  day.isToday ? "text-accent" : "text-ink"
                )}
                aria-current={day.isToday ? "date" : undefined}
              >
                {day.weekday}{" "}
                <span className="tabular-nums">{day.dayNumber}</span>
              </h2>
              {day.nutrition && (
                <DayMeter
                  day={day.nutrition}
                  targets={targets}
                  className="flex-1"
                />
              )}
            </header>
            <ul className="flex flex-col gap-1.5">
              {slotRows.map((row) => {
                const cell = day.cells.find((c) => c.slotId === row.slotId)
                if (!cell) return null
                return (
                  <li
                    key={cell.key}
                    className="grid grid-cols-[5.5rem_1fr] items-start gap-2"
                  >
                    <span className="pt-2 font-heading text-small text-ink-muted italic">
                      {row.name}
                    </span>
                    <PlateCell
                      cell={cell}
                      layout="list"
                      readOnly={board.readOnly}
                      dnd={false}
                      onOpen={onOpenCell}
                    />
                  </li>
                )
              })}
            </ul>
          </section>
        )
      })}
    </section>
  )
}
