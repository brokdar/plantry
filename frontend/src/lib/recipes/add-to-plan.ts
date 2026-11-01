import type { Plan, PlanSlot } from "@/lib/api/plan"
import { mainComponent } from "@/lib/plan/board"

/** A cell a recipe can be added to: a `plan` cell without a plate, or one
 *  holding only a draft plate, which the new plate replaces. */
export interface OpenCell {
  /** `date|slotId`, as in the board's `?cell`. */
  key: string
  date: string
  slot: PlanSlot
  /** The window holding the cell, for "Open plan". */
  windowStart: string
  /** The draft plate the new plate replaces; null on an empty cell. */
  replaces: { plateId: number; name: string | null } | null
}

interface OpenDay {
  date: string
  cells: OpenCell[]
}

/** The `plan` cells of the given windows from today on that are empty or
 *  hold a draft plate, grouped by day in date and meal order (D36). After a
 *  generate every cell holds a draft, so those are offered too, marked as
 *  replacing it. No-plate, routine and confirmed cells are skipped. */
export function openCells(plans: readonly Plan[]): OpenDay[] {
  const days = new Map<string, OpenCell[]>()
  for (const plan of plans) {
    const slots = new Map(plan.slots.map((s) => [s.id, s]))
    for (const cell of plan.cells) {
      const slot = slots.get(cell.slot_id)
      if (
        !slot ||
        cell.mode !== "plan" ||
        cell.plate?.state === "confirmed" ||
        cell.date < plan.window.today
      )
        continue
      const list = days.get(cell.date) ?? []
      list.push({
        key: `${cell.date}|${cell.slot_id}`,
        date: cell.date,
        slot,
        windowStart: plan.window.start,
        replaces: cell.plate
          ? {
              plateId: cell.plate.id,
              name: mainComponent(cell.plate.components)?.food_name ?? null,
            }
          : null,
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
