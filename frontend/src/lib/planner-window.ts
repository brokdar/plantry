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

function padTwo(n: number): string {
  return String(n).padStart(2, "0")
}

function toYMD(d: Date): string {
  return `${d.getFullYear()}-${padTwo(d.getMonth() + 1)}-${padTwo(d.getDate())}`
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
