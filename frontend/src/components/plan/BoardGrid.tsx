import { useState, type KeyboardEvent } from "react"
import { useTranslation } from "react-i18next"

import { DayMeter } from "@/components/batch/DayMeter"
import type { PlanTargets } from "@/lib/api/plan"
import type { Board, BoardCell } from "@/lib/plan/board"
import { handleGridArrowKey, readCellPos } from "@/lib/plan/keynav"
import { cn } from "@/lib/utils"

import { PlateCell } from "./PlateCell"

export interface BoardLayoutProps {
  board: Board
  targets: PlanTargets | null
  /** Accessible name of the board ("Meal plan, 31 Oct – 6 Nov"). */
  label: string
  onOpenCell: (cell: BoardCell) => void
  /** Keyboard N on a cell. */
  onMarkNoPlate: (cell: BoardCell) => void
}

/**
 * The board as a real ARIA grid (UX §5.5, §8): days as columns starting on
 * the shopping day, meals as rows, with column and row headers. From 1024 px
 * all seven days fit; between 768 and 1023 px four days show and the rest
 * swipe in, snapping per day, with the day headers and the meal column
 * sticky. Arrow keys move between cells (one tab stop), Enter opens, N marks
 * No plate.
 */
export function BoardGrid({
  board,
  targets,
  label,
  onOpenCell,
  onMarkNoPlate,
}: BoardLayoutProps) {
  const { t } = useTranslation()
  const rows = board.rows.filter((r) => !r.collapsed)
  const [focusKey, setFocusKey] = useState<string | null>(null)
  const tabKey =
    focusKey && rows.some((r) => r.cells.some((c) => c.key === focusKey))
      ? focusKey
      : rows[0]?.cells[0]?.key

  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    if (handleGridArrowKey(e, { rows: rows.length, cols: board.days.length }))
      return
    if (e.key !== "n" && e.key !== "N") return
    if (e.metaKey || e.ctrlKey || e.altKey || board.readOnly) return
    const pos = readCellPos(e.target)
    const cell = pos ? rows[pos.row]?.cells[pos.col] : undefined
    if (!cell || cell.mode === "none") return
    e.preventDefault()
    onMarkNoPlate(cell)
  }

  return (
    // The scroll box: below 1024 px it scrolls sideways (4 days visible)
    // and vertically, so the day headers can stick inside it.
    <div className="@container max-h-[70dvh] snap-x snap-mandatory scroll-pl-24 overflow-auto overscroll-x-contain lg:max-h-none lg:overflow-visible">
      <div
        role="grid"
        aria-label={label}
        aria-readonly={board.readOnly || undefined}
        onKeyDown={onKeyDown}
        className="grid grid-cols-[6rem_repeat(7,calc((100cqw-7rem)/4))] gap-1 p-1 lg:grid-cols-[7rem_repeat(7,minmax(0,1fr))]"
      >
        <div role="row" className="contents">
          <div
            role="columnheader"
            className="sticky top-0 left-0 z-20 bg-ground"
          >
            <span className="sr-only">{t("plan.board.meal_column")}</span>
          </div>
          {board.days.map((day) => (
            <div
              key={day.date}
              role="columnheader"
              aria-label={`${day.weekday} ${day.dayNumber}`}
              className="sticky top-0 z-10 flex snap-start flex-col gap-0.5 bg-ground px-1 pt-1"
            >
              <span
                className={cn(
                  "flex items-baseline gap-1.5 text-small",
                  day.isToday ? "font-semibold text-accent" : "text-ink-muted"
                )}
                aria-current={day.isToday ? "date" : undefined}
              >
                <span>{day.weekdayShort}</span>
                <span className="text-body text-ink tabular-nums">
                  {day.dayNumber}
                </span>
              </span>
              {day.nutrition && (
                <DayMeter day={day.nutrition} targets={targets} />
              )}
            </div>
          ))}
        </div>

        {board.rows.map((row) =>
          row.collapsed ? (
            <div key={row.slotId} role="row" className="contents">
              <div
                role="rowheader"
                className="sticky left-0 z-10 flex items-center bg-ground pr-2 font-heading text-body text-ink-muted italic"
              >
                {row.name}
              </div>
              <div
                role="gridcell"
                aria-colspan={board.days.length}
                className="col-span-7 flex min-h-11 items-center rounded-cell px-2 text-small text-ink-muted"
              >
                {row.collapsed}
              </div>
            </div>
          ) : (
            <div key={row.slotId} role="row" className="contents">
              <div
                role="rowheader"
                className="sticky left-0 z-10 flex items-start bg-ground pt-2 pr-2 font-heading text-body text-ink-muted italic"
              >
                {row.name}
              </div>
              {row.cells.map((cell, ci) => (
                <PlateCell
                  key={cell.key}
                  cell={cell}
                  layout="grid"
                  readOnly={board.readOnly}
                  dnd
                  onOpen={onOpenCell}
                  pos={`${rows.indexOf(row)}-${ci}`}
                  tabStop={cell.key === tabKey}
                  onFocusCell={setFocusKey}
                />
              ))}
            </div>
          )
        )}
      </div>
    </div>
  )
}
