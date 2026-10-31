import type { Plan, PlanSlot } from "@/lib/api/plan"

/** A cell a recipe can be added to: a `plan` cell without a plate. */
export interface OpenCell {
  /** `date|slotId`, as in the board's `?cell`. */
  key: string
  date: string
  slot: PlanSlot
  /** The window holding the cell, for "Open plan". */
  windowStart: string
}

interface OpenDay {
  date: string
  cells: OpenCell[]
}

/** The empty `plan` cells of the given windows from today on, grouped by
 *  day in date and meal order (D36). No-plate, routine and occupied cells
 *  are skipped; the first cell is the preselection. */
export function openCells(plans: readonly Plan[]): OpenDay[] {
  const days = new Map<string, OpenCell[]>()
  for (const plan of plans) {
    const slots = new Map(plan.slots.map((s) => [s.id, s]))
    for (const cell of plan.cells) {
      const slot = slots.get(cell.slot_id)
      if (
        !slot ||
        cell.mode !== "plan" ||
        cell.plate ||
        cell.date < plan.window.today
      )
        continue
      const list = days.get(cell.date) ?? []
      list.push({
        key: `${cell.date}|${cell.slot_id}`,
        date: cell.date,
        slot,
        windowStart: plan.window.start,
      })
      days.set(cell.date, list)
    }
  }
  return [...days.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([date, cells]) => ({
      date,
      cells: cells.sort((a, b) => a.slot.sort_order - b.slot.sort_order),
    }))
}
