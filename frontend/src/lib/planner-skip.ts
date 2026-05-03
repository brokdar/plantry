import {
  createPlate,
  type Plate,
  type SetPlateSkippedInput,
} from "@/lib/api/plates"
import { plateKeys } from "@/lib/queries/keys"
import { queryClient } from "@/lib/query-client"

export interface ToggleSkipArgs {
  date: string
  slotId: number
  existing: Plate | undefined
  noteOverride?: string | null
  rangeFrom: string
  rangeTo: string
  setSkipped: (args: {
    plateId: number
    input: SetPlateSkippedInput
  }) => Promise<unknown>
}

/**
 * Compute and persist the next skip state for a slot. Used by both the
 * desktop and mobile planner grids so the semantics of `noteOverride`
 * stay in one place.
 *
 * - `noteOverride === undefined` → a plain toggle that preserves the existing
 *   note when going false→true. When unsetting (true→false), the note is
 *   cleared so a later re-skip starts blank — without this, an old note
 *   would silently resurface and surprise the user.
 * - `noteOverride === string | null` → set/clear the note explicitly. When
 *   the slot is already skipped, this is treated as "edit note" — skipped
 *   stays true.
 * - When no plate exists yet (empty cell), one is created first.
 */
export async function toggleSkip({
  date,
  slotId,
  existing,
  noteOverride,
  rangeFrom,
  rangeTo,
  setSkipped,
}: ToggleSkipArgs): Promise<void> {
  let id = existing?.id ?? null
  if (id === null) {
    const created = await createPlate({ date, slot_id: slotId })
    id = created.id
    void queryClient.invalidateQueries({
      queryKey: plateKeys.range(rangeFrom, rangeTo),
    })
  }

  const wasSkipped = !!existing?.skipped
  const editingNote = noteOverride !== undefined && wasSkipped
  const nextSkipped = editingNote ? true : !wasSkipped

  let nextNote: string | null
  if (noteOverride !== undefined) {
    nextNote = noteOverride
  } else if (wasSkipped && !nextSkipped) {
    // Unskip — clear note so the next re-skip starts blank.
    nextNote = null
  } else {
    nextNote = existing?.note ?? null
  }

  await setSkipped({
    plateId: id,
    input: { skipped: nextSkipped, note: nextNote },
  })
}
