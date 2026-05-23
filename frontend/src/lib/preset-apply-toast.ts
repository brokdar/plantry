import type { TFunction } from "i18next"

import type { ApplyResult, ApplySnapshot } from "@/lib/api/presets"
import { toast } from "@/lib/toast"

/**
 * Render the toast sequence for a preset apply / copy-week result.
 *
 * - Success line summarises Created / Replaced.
 * - Warning lines surface SkippedOccupied / SkippedNoSlot when non-empty.
 * - When the result includes Replaced plates, the success toast carries an
 *   Undo action that calls `onUndo` with the server-side snapshot.
 */
export function showPresetApplyToasts(
  result: ApplyResult,
  t: TFunction,
  onUndo: (snapshot: ApplySnapshot) => void
) {
  const created = result.created.length
  const replaced = result.replaced.length

  if (created + replaced === 0) {
    if (result.skipped_occupied.length > 0) {
      toast.warning(
        t("preset.apply.result_skipped_occupied", {
          count: result.skipped_occupied.length,
        })
      )
      return
    }
    if (result.skipped_no_slot.length > 0) {
      toast.warning(
        t("preset.apply.result_skipped_no_slot", {
          count: result.skipped_no_slot.length,
        })
      )
      return
    }
    return
  }

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
        onClick: () => onUndo(result.snapshot),
      },
    })
  } else {
    toast.success(message)
  }

  if (result.skipped_occupied.length > 0) {
    toast.warning(
      t("preset.apply.result_skipped_occupied", {
        count: result.skipped_occupied.length,
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
