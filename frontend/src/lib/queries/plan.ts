import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  clearAvailabilityOverride,
  setAvailabilityOverride,
} from "@/lib/api/availability"
import { clearPlan, getPlan, restorePlan, type PlanCell } from "@/lib/api/plan"
import type { PlanSnapshot, RemovedResponse } from "@/lib/api/plates"
import { toast, toastError } from "@/lib/toast"

import { availabilityKeys, planKeys, shoppingKeys, todayKeys } from "./keys"

/** Every plan mutation refreshes the read models and the shopping list. */
export function invalidatePlanViews(qc: QueryClient) {
  void qc.invalidateQueries({ queryKey: planKeys.all })
  void qc.invalidateQueries({ queryKey: todayKeys.all })
  void qc.invalidateQueries({ queryKey: shoppingKeys.all })
}

/** The board's read model. Without `start` the server opens the anchored
 *  window. */
export function usePlan(start?: string) {
  return useQuery({
    queryKey: planKeys.window(start),
    queryFn: () => getPlan(start),
  })
}

export function useRestorePlan() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (snapshot: PlanSnapshot) => restorePlan(snapshot),
    onSuccess: () => invalidatePlanViews(qc),
    onError: (err) => toastError(err, t),
  })
}

/** How long Undo stays on offer after a removal (UX §6). */
export const UNDO_MS = 6000

export type RemovalVerb = "plate_removed" | "batch_removed" | "plan_cleared"

const REMOVAL_MESSAGE_KEYS: Record<RemovalVerb, string> = {
  plate_removed: "plan.undo.plate_removed",
  batch_removed: "plan.undo.batch_removed",
  plan_cleared: "plan.undo.plan_cleared",
}

/**
 * The single undo helper for removing mutations (D22). The returned function
 * takes the `{removed}` body, shows a 6 s toast named after what happened
 * and, on Undo, restores the snapshot. Undo works once and not after the
 * toast has closed. An empty snapshot offers nothing.
 */
export function useUndoableRemoval() {
  const { t } = useTranslation()
  const restore = useRestorePlan()

  return function offerUndo({ removed }: RemovedResponse, verb: RemovalVerb) {
    if (removed.plates.length === 0 && removed.sessions.length === 0) return
    let closed = false
    const close = () => {
      closed = true
    }
    toast(t(REMOVAL_MESSAGE_KEYS[verb]), {
      duration: UNDO_MS,
      action: {
        label: t("common.undo"),
        onClick: () => {
          if (closed) return
          closed = true
          restore.mutate(removed)
        },
      },
      onDismiss: close,
      onAutoClose: close,
    })
  }
}

/** Clear window: removes every plate in [from, to] and offers Undo. */
export function useClearWindow() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const offerUndo = useUndoableRemoval()
  return useMutation({
    mutationFn: ({ from, to }: { from: string; to: string }) =>
      clearPlan(from, to),
    onSuccess: (res) => {
      invalidatePlanViews(qc)
      offerUndo(res, "plan_cleared")
    },
    onError: (err) => toastError(err, t),
  })
}

/**
 * "Mark as No plate" on a board cell (keyboard N): sets a `none` override,
 * removing the cell's plate, and offers Undo for UNDO_MS. Undo puts the
 * cell's previous override back (or clears it) and restores the plate with
 * its batch from the returned snapshot.
 */
export function useMarkNoPlate() {
  const qc = useQueryClient()
  const { t } = useTranslation()

  function refresh() {
    invalidatePlanViews(qc)
    void qc.invalidateQueries({ queryKey: availabilityKeys.ranges() })
  }

  async function undo(cell: PlanCell, removed: PlanSnapshot | null) {
    try {
      if (cell.source === "override") {
        await setAvailabilityOverride(cell.date, cell.slot_id, {
          mode: cell.mode,
          preset_id: cell.preset_id,
          note: cell.note,
        })
      } else {
        await clearAvailabilityOverride(cell.date, cell.slot_id)
      }
      if (removed) await restorePlan(removed)
    } catch (err) {
      toastError(err, t)
    } finally {
      refresh()
    }
  }

  return useMutation({
    mutationFn: (cell: PlanCell) =>
      setAvailabilityOverride(cell.date, cell.slot_id, {
        mode: "none",
        note: cell.note,
        remove_plate: cell.plate !== null,
      }),
    onSuccess: (res, cell) => {
      refresh()
      let used = false
      toast(t("plan.undo.marked_no_plate"), {
        duration: UNDO_MS,
        action: {
          label: t("common.undo"),
          onClick: () => {
            if (used) return
            used = true
            void undo(cell, res.removed)
          },
        },
      })
    },
    onError: (err) => toastError(err, t),
  })
}
