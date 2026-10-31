import type { TFunction } from "i18next"

import type { ApplyResult } from "@/lib/api/presets"
import { toast } from "@/lib/toast"

/**
 * Render the toast sequence for a preset apply / copy-week result.
 *
 * - Success line summarises Created / Replaced (the plates `removed` holds).
 * - Warning lines surface SkippedOccupied / SkippedUnavailable (No-plate
 *   cells, D12) / SkippedNoSlot when non-empty.
 * - When plates were replaced, the success toast carries an Undo action that
 *   calls `onUndo` with the result.
 */
export function showPresetApplyToasts(
  result: ApplyResult,
  t: TFunction,
  onUndo: (result: ApplyResult) => void
) {
  const replaced = result.removed.plates.length
  const created = Math.max(0, result.created_plate_ids.length - replaced)

  if (created + replaced > 0) {
    const messages: string[] = []
    if (created > 0) {
      messages.push(t("preset.apply.result_created", { count: created }))
    }
    if (replaced > 0) {
      messages.push(t("preset.apply.result_replaced", { count: replaced }))
    }
    const message = messages.join(" ")

    if (replaced > 0) {
      toast.success(message, {
        action: {
          label: t("preset.apply.undo"),
          onClick: () => onUndo(result),
        },
      })
    } else {
      toast.success(message)
    }
  }

  if (result.skipped_occupied.length > 0) {
    toast.warning(
      t("preset.apply.result_skipped_occupied", {
        count: result.skipped_occupied.length,
      })
    )
  }
  if (result.skipped_unavailable.length > 0) {
    toast.warning(
      t("preset.apply.result_skipped_unavailable", {
        count: result.skipped_unavailable.length,
      })
    )
  }
  if (result.skipped_no_slot.length > 0) {
    toast.warning(
      t("preset.apply.result_skipped_no_slot", {
        count: result.skipped_no_slot.length,
      })
    )
  }
}
