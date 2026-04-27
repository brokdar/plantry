import {
  addDays,
  format,
  getDay,
  getDaysInMonth,
  isSameMonth,
  isToday,
  startOfMonth,
} from "date-fns"
import { useTranslation } from "react-i18next"

import type { Food } from "@/lib/api/foods"
import type { Plate } from "@/lib/api/plates"
import { cn } from "@/lib/utils"

import { MonthCell } from "./MonthCell"

interface MonthGridProps {
  year: number
  month: number // 0-based
  weekStartsOn: 0 | 1 | 6
  plates: Plate[]
  search: string
  onCellClick: (date: string) => void
  foodsById?: Map<number, Food>
}

/** Returns the 7 column indices (0=Sun … 6=Sat) ordered by weekStartsOn. */
function buildColumnOrder(weekStartsOn: 0 | 1 | 6): number[] {
  const cols: number[] = []
  let d = weekStartsOn
  for (let i = 0; i < 7; i++) {
    cols.push(d % 7)
    d++
  }
  return cols
}

/** Returns the grid start date — the weekStartsOn-aligned day that contains
 *  the first of the month (may be in the previous month). */
function gridStart(year: number, month: number, weekStartsOn: 0 | 1 | 6): Date {
  const first = startOfMonth(new Date(year, month, 1))
  const dow = getDay(first) // 0=Sun … 6=Sat
  // How many days to go back from `first` to reach the previous weekStartsOn day?
  const diff = (dow - weekStartsOn + 7) % 7
  return addDays(first, -diff)
}

/** Returns YYYY-MM-DD for a Date. */
function toISODate(d: Date): string {
  return format(d, "yyyy-MM-dd")
}

/** Check if a plate matches the search string (against note or food names). */
function plateMatchesSearch(
  plate: Plate,
  q: string,
  foodsById?: Map<number, Food>
): boolean {
  if (!q) return true
  const lower = q.toLowerCase()
  if (plate.note?.toLowerCase().includes(lower)) return true
  return plate.components.some((c) =>
    foodsById?.get(c.food_id)?.name.toLowerCase().includes(lower)
  )
}

export function MonthGrid({
  year,
  month,
  weekStartsOn,
  plates,
  search,
  onCellClick,
  foodsById,
}: MonthGridProps) {
  const { i18n } = useTranslation()
  const columnOrder = buildColumnOrder(weekStartsOn)
  const start = gridStart(year, month, weekStartsOn)

  // Build date rows: 5 or 6 weeks
  const daysInMonth = getDaysInMonth(new Date(year, month, 1))
  const gridDays =
    Math.ceil(
      (((getDay(startOfMonth(new Date(year, month, 1))) - weekStartsOn + 7) %
        7) +
        daysInMonth) /
        7
    ) * 7

  const dates: Date[] = []
  for (let i = 0; i < gridDays; i++) {
    dates.push(addDays(start, i))
  }

  // Index plates by date string
  const platesByDate = new Map<string, Plate[]>()
  for (const p of plates) {
    const key = p.date.slice(0, 10)
    if (!platesByDate.has(key)) platesByDate.set(key, [])
    platesByDate.get(key)!.push(p)
  }

  const monthDate = new Date(year, month, 1)
  const hasSearch = search.trim().length > 0

  const rows = dates.length / 7

  return (
    <div className="flex flex-col gap-px">
      {/* Column headers — locale-aware day abbreviations */}
      <div className="mb-1 grid grid-cols-7 gap-px">
        {columnOrder.map((dow) => {
          // Use a fixed reference Sunday + offset to get the right weekday
          const refDate = new Date(2000, 0, 2 + dow) // 2000-01-02 is a Sunday
          const label = new Intl.DateTimeFormat(i18n.language, {
            weekday: "short",
          }).format(refDate)
          return (
            <div
              key={dow}
              className="py-1 text-center font-heading text-[10px] font-bold tracking-widest text-on-surface-variant uppercase"
            >
              {label}
            </div>
          )
        })}
      </div>

      {/* Grid rows */}
      {Array.from({ length: rows }, (_, row) => (
        <div key={row} className={cn("grid grid-cols-7 gap-px")}>
          {columnOrder.map((_, colIdx) => {
            const date = dates[row * 7 + colIdx]
            const iso = toISODate(date)
            const cellPlates = platesByDate.get(iso) ?? []
            const inMonth = isSameMonth(date, monthDate)

            let matchesSearch: boolean | null = null
            if (hasSearch) {
              matchesSearch = cellPlates.some((p) =>
                plateMatchesSearch(p, search, foodsById)
              )
            }

            return (
              <MonthCell
                key={iso}
                date={iso}
                plates={cellPlates}
                isCurrentMonth={inMonth}
                isToday={isToday(date)}
                matchesSearch={matchesSearch}
                onClick={onCellClick}
                foodsById={foodsById}
              />
            )
          })}
        </div>
      ))}
    </div>
  )
}
