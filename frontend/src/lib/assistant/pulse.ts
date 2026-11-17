import type { Change } from "@/lib/domain/assistantEvents"
import type { BoardCell } from "@/lib/plan/board"

/** Whether the board cell belongs to the change (D17): its plate or one of
 *  its sessions was written, or it is empty on a date the change touched (a
 *  move's source cell, an away cell), or, for a change with no rows (an
 *  availability edit), its date. */
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
  if (!change.dates.includes(cell.date)) return false
  const rows = change.plate_ids.length + change.session_ids.length
  return rows === 0 || !cell.plate
}
