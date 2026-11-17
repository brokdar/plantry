import type { Change } from "@/lib/domain/assistantEvents"
import type { BoardCell } from "@/lib/plan/board"

/** The `date|slotId` of every cell the change's ops name: an op's `pulse`
 *  list (a move's source and target, away cells, copy targets, a cook's old
 *  and new cell, a deleted plate's cell), or its own `date` + `slot_id` (an
 *  availability edit, a created plate). */
function opCells(change: Change): Set<string> {
  const out = new Set<string>()
  const ops = Array.isArray(change.params.ops)
    ? (change.params.ops as Record<string, unknown>[])
    : []
  for (const op of ops) {
    if (Array.isArray(op.pulse)) {
      for (const c of op.pulse as Record<string, unknown>[]) {
        if (typeof c?.date === "string" && typeof c.slot_id === "number") {
          out.add(`${c.date}|${c.slot_id}`)
        }
      }
    } else if (typeof op.date === "string" && typeof op.slot_id === "number") {
      out.add(`${op.date}|${op.slot_id}`)
    }
  }
  return out
}

/** Whether the board cell belongs to the change (D17): its plate or one of
 *  its sessions was written, or it is one of the exact cells the change's ops
 *  name (a move's source cell, an away cell). A change whose ops name no cell
 *  (rows written before they did) falls back to its dates for an empty cell,
 *  or for any cell when the change has no rows. */
export function cellInChange(cell: BoardCell, change: Change | null): boolean {
  if (!change) return false
  if (cell.plate && change.plate_ids.includes(cell.plate.id)) return true
  if (
    [...cell.sessionIds, ...cell.cookOf].some((id) =>
      change.session_ids.includes(id)
    )
  ) {
    return true
  }
  const cells = opCells(change)
  if (cells.size > 0) return cells.has(`${cell.date}|${cell.slotId}`)
  if (!change.dates.includes(cell.date)) return false
  const rows = change.plate_ids.length + change.session_ids.length
  return rows === 0 || !cell.plate
}
