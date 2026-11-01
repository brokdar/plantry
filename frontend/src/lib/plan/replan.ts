import type { PlanWindow } from "@/lib/api/plan"

/**
 * The first day a re-plan of this window may touch (§3.2: `from` ≥ today):
 * today, or the window's start when the window lies ahead; null for a past
 * window.
 */
export function replanFrom(win: PlanWindow): string | null {
  if (win.end < win.today) return null
  return win.start > win.today ? win.start : win.today
}
