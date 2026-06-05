import { addDays, getDaysInMonth } from "date-fns"

export type AnchorMode = "today" | "next_shopping_day" | "fixed_weekday"

/**
 * Convert backend weekday (0=Monday…6=Sunday) to JS Date.getDay() (0=Sunday…6=Saturday).
 */
function toJsDay(backendDay: number): number {
  return (backendDay + 1) % 7
}

function midnight(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate())
}

function nextOccurrence(from: Date, jsTargetDay: number): Date {
  const base = midnight(from)
  const current = base.getDay()
  if (current === jsTargetDay) return base
  const diff = (jsTargetDay - current + 7) % 7
  return new Date(base.getFullYear(), base.getMonth(), base.getDate() + diff)
}

export function computeAnchor(opts: {
  mode: AnchorMode
  shoppingDay: number // 0=Monday…6=Sunday (backend convention)
  fixedWeekday?: number // 0=Monday…6=Sunday (backend convention)
  weekStartsOn: "monday" | "sunday" | "saturday"
  today?: Date // defaults to new Date() — injectable for tests
}): Date {
  const today = midnight(opts.today ?? new Date())

  if (opts.mode === "today") {
    return today
  }

  if (opts.mode === "next_shopping_day") {
    return nextOccurrence(today, toJsDay(opts.shoppingDay))
  }

  // fixed_weekday
  const jsFixed = toJsDay(opts.fixedWeekday ?? 0)
  return nextOccurrence(today, jsFixed)
}

export function padTwo(n: number): string {
  return String(n).padStart(2, "0")
}

export function toYMD(d: Date): string {
  return `${d.getFullYear()}-${padTwo(d.getMonth() + 1)}-${padTwo(d.getDate())}`
}

/** Shifts a YYYY-MM-DD by `days` (positive or negative). DST-safe because the
 *  Date constructor normalizes overflow at midnight. */
export function shiftYMD(ymd: string, days: number): string {
  const [y, m, d] = ymd.split("-").map(Number)
  const dt = new Date(y, (m ?? 1) - 1, (d ?? 1) + days)
  return toYMD(dt)
}

export function windowRange(
  anchor: Date,
  days: number
): { from: string; to: string } {
  const from = midnight(anchor)
  const to = new Date(
    from.getFullYear(),
    from.getMonth(),
    from.getDate() + days - 1
  )
  return { from: toYMD(from), to: toYMD(to) }
}

export function todayISO(): string {
  return toYMD(new Date())
}

export function currentMonthISO(): string {
  const now = new Date()
  return `${now.getFullYear()}-${padTwo(now.getMonth() + 1)}`
}

export function weekStartDate(weekStartsOn: 0 | 1 | 6): string {
  const today = new Date()
  const dow = today.getDay() // 0=Sun…6=Sat
  const diff = (dow - weekStartsOn + 7) % 7
  return toYMD(addDays(today, -diff))
}

/** Parse a YYYY-MM date string to { year, month (0-based) }. */
export function parseYearMonth(s: string): { year: number; month: number } {
  const [y, m] = s.split("-").map(Number)
  return { year: y, month: (m ?? 1) - 1 }
}

/** Compute the visible grid range for a month view (partial leading/trailing weeks). */
export function monthGridRange(
  year: number,
  month: number,
  weekStartsOn: 0 | 1 | 6
): { from: string; to: string } {
  const firstOfMonth = new Date(year, month, 1)
  const dow = firstOfMonth.getDay()
  const startDiff = (dow - weekStartsOn + 7) % 7
  const gridStart = addDays(firstOfMonth, -startDiff)
  const daysInMonth = getDaysInMonth(firstOfMonth)
  const totalGridDays = Math.ceil((startDiff + daysInMonth) / 7) * 7
  return {
    from: toYMD(gridStart),
    to: toYMD(addDays(gridStart, totalGridDays - 1)),
  }
}
