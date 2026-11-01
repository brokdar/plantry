function pad(n: number): string {
  return String(n).padStart(2, "0")
}

/** A local date as YYYY-MM-DD. */
function toYMD(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/** The client's local date as YYYY-MM-DD. Every endpoint that depends on the
 *  current date gets it as `today` (D25). */
export function todayISO(): string {
  return toYMD(new Date())
}

/** Shifts a YYYY-MM-DD by `days` (positive or negative). DST-safe because the
 *  Date constructor normalizes overflow at local midnight. */
export function shiftYMD(ymd: string, days: number): string {
  const [y, m, d] = ymd.split("-").map(Number)
  return toYMD(new Date(y, m - 1, d + days))
}

/** The start of the plan window holding `ymd`: the latest shopping day
 *  (0 = Monday … 6 = Sunday) on or before it, like the server's
 *  `window.Containing`. */
export function windowStartOf(ymd: string, shoppingDay: number): string {
  const [y, m, d] = ymd.split("-").map(Number)
  const weekday = (new Date(y, m - 1, d).getDay() + 6) % 7
  return shiftYMD(ymd, -((weekday - shoppingDay + 7) % 7))
}
