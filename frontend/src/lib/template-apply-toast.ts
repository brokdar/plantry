import type { TFunction } from "i18next"

import {
  addPlateComponent,
  createPlate,
  deletePlate,
  setPlateSkipped,
  type Plate,
} from "@/lib/api/plates"
import { putFeedback } from "@/lib/api/feedback"
import { plateKeys } from "@/lib/queries/keys"
import { queryClient } from "@/lib/query-client"
import { toast, toastError } from "@/lib/toast"
import type { TemplatePickerApplyInfo } from "@/components/templates/TemplatePicker"

/** Captures the plates currently occupying the given keys from the planner
 * range cache. Returns a frozen copy the caller can use to restore on undo. */
export function snapshotOverwrittenPlates(
  rangeFrom: string,
  rangeTo: string,
  overwrittenKeys: string[]
): Plate[] {
  if (overwrittenKeys.length === 0) return []
  const data = queryClient.getQueryData<{ plates: Plate[] }>(
    plateKeys.range(rangeFrom, rangeTo)
  )
  const plates = data?.plates ?? []
  const wanted = new Set(overwrittenKeys)
  return plates
    .filter((p) => wanted.has(`${p.date}|${p.slot_id}`))
    .map((p) => structuredClone(p))
}

/** Re-creates the snapshotted plates and re-attaches their components/skip
 * state. Best-effort: failures are surfaced as toasts but never block.
 *
 * `replacementIds` are the IDs of newly-created plates that landed on the
 * snapshot's slots — those are the only "Replaced" plates we delete. Plates
 * the same apply created on previously-empty slots are intentionally left
 * alone: undoing a "Replaced N plates" toast should reverse the replacement,
 * not the rest of the apply. */
async function restoreOverwrittenPlates(
  snapshot: Plate[],
  rangeFrom: string,
  rangeTo: string,
  replacementIds: number[],
  t: TFunction
): Promise<void> {
  await Promise.allSettled(replacementIds.map((id) => deletePlate(id)))
  for (const snap of snapshot) {
    try {
      const created = await createPlate({
        date: snap.date,
        slot_id: snap.slot_id,
        note: snap.note ?? undefined,
      })
      if (snap.skipped) {
        await setPlateSkipped(created.id, {
          skipped: true,
          note: snap.note ?? null,
        })
        continue
      }
      for (const pc of snap.components) {
        await addPlateComponent(created.id, {
          food_id: pc.food_id,
          // TODO(plate-workflow-rework, phase 3): pass kind-aware quantity.
          portions: pc.portions ?? 1,
        })
      }
      if (snap.feedback?.status) {
        await putFeedback(created.id, {
          status: snap.feedback.status,
          note: snap.feedback.note ?? null,
        })
      }
    } catch (err) {
      toastError(err, t)
    }
  }
  await queryClient.invalidateQueries({
    queryKey: plateKeys.range(rangeFrom, rangeTo),
  })
  void queryClient.invalidateQueries({ queryKey: ["nutrition"] })
}

/** Surfaces toasts for an apply result. When plates were overwritten, the
 * "Replaced N plates" toast carries an Undo action that restores the
 * pre-apply snapshot. */
export function showApplyToasts(
  info: TemplatePickerApplyInfo,
  snapshot: Plate[],
  rangeFrom: string,
  rangeTo: string,
  t: TFunction
): void {
  const skipped = info.skipped.length
  const overwritten = snapshot.length
  // Split the created list into "filled a previously-empty slot" vs
  // "replaced an existing plate" so each gets the right toast and only
  // the replacements are reversible by Undo.
  const overwrittenKeys = new Set(snapshot.map((p) => `${p.date}|${p.slot_id}`))
  const replacementIds: number[] = []
  let freshFills = 0
  for (const c of info.created) {
    if (overwrittenKeys.has(`${c.date}|${c.slot_id}`)) {
      replacementIds.push(c.id)
    } else {
      freshFills++
    }
  }

  if (freshFills > 0) {
    toast.success(t("template.apply_result", { count: freshFills }))
  }
  if (skipped > 0) {
    toast(t("template.apply_result_skipped", { count: skipped }))
  }
  if (overwritten > 0) {
    toast(t("template.apply_overwritten", { count: overwritten }), {
      action: {
        label: t("template.apply_undo_label"),
        onClick: () => {
          void restoreOverwrittenPlates(
            snapshot,
            rangeFrom,
            rangeTo,
            replacementIds,
            t
          )
        },
      },
      duration: 8000,
    })
  }
}
