import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  clearAvailabilityOverride,
  getAvailabilityRange,
  setAvailabilityOverride,
} from "@/lib/api/availability"
import { clearPlan, getPlan, restorePlan, type PlanCell } from "@/lib/api/plan"
import type { PlanSnapshot, RemovedResponse } from "@/lib/api/plates"
import { toast, toastError } from "@/lib/toast"

import { availabilityKeys, planKeys, shoppingKeys, todayKeys } from "./keys"

/** Every plan mutation refreshes the read models and the shopping list.
 *  Returned from `onSuccess`, the mutation stays pending until the active
 *  views have refetched, so a control showing a pending value never flashes
 *  back to the old one. */
export function invalidatePlanViews(qc: QueryClient): Promise<unknown> {
  return Promise.all([
    qc.invalidateQueries({ queryKey: planKeys.all }),
    qc.invalidateQueries({ queryKey: todayKeys.all }),
    qc.invalidateQueries({ queryKey: shoppingKeys.all }),
  ])
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
 * The undo toast every undoable plan action shares (UX §6): the returned
 * function shows `message` for UNDO_MS with an Undo action that runs `undo`
 * once, and not after the toast has closed.
 */
export function useUndoToast() {
  const { t } = useTranslation()
  return function offer(message: string, undo: () => void) {
    let closed = false
    const close = () => {
      closed = true
    }
    toast(message, {
      duration: UNDO_MS,
      action: {
        label: t("common.undo"),
        onClick: () => {
          if (closed) return
          closed = true
          undo()
        },
      },
      onDismiss: close,
      onAutoClose: close,
    })
  }
}

/**
 * The single undo helper for removing mutations (D22). The returned function
 * takes the `{removed}` body, shows the undo toast named after what happened
 * and, on Undo, restores the snapshot. An empty snapshot offers nothing.
 */
export function useUndoableRemoval() {
  const { t } = useTranslation()
  const restore = useRestorePlan()
  const offer = useUndoToast()

  return function offerUndo({ removed }: RemovedResponse, verb: RemovalVerb) {
    if (removed.plates.length === 0 && removed.sessions.length === 0) return
    offer(t(REMOVAL_MESSAGE_KEYS[verb]), () => restore.mutate(removed))
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
      void invalidatePlanViews(qc)
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
    void invalidatePlanViews(qc)
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
      // Undo works once, and not after the toast has closed.
      let closed = false
      const close = () => {
        closed = true
      }
      toast(t("plan.undo.marked_no_plate"), {
        duration: UNDO_MS,
        action: {
          label: t("common.undo"),
          onClick: () => {
            if (closed) return
            closed = true
            void undo(cell, res.removed)
          },
        },
        onDismiss: close,
        onAutoClose: close,
      })
    },
    onError: (err) => toastError(err, t),
  })
}

/**
 * "Plan this meal" on a No plate cell. A date override is dropped so the
 * cell falls back to its weekday default; only when that default is `none`
 * too does it get an explicit `plan` override, so dates are not pinned
 * needlessly.
 */
export function usePlanMeal() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: async (cell: PlanCell) => {
      if (cell.source === "override") {
        await clearAvailabilityOverride(cell.date, cell.slot_id)
        const { items } = await getAvailabilityRange(cell.date, cell.date)
        const resolved = items.find((c) => c.slot_id === cell.slot_id)
        if (resolved?.mode !== "none") return
      }
      await setAvailabilityOverride(cell.date, cell.slot_id, { mode: "plan" })
    },
    onSuccess: () => {
      void invalidatePlanViews(qc)
      void qc.invalidateQueries({ queryKey: availabilityKeys.ranges() })
    },
    onError: (err) => toastError(err, t),
  })
}

/** The reason on a No plate cell ("Eating out"); empty clears it. */
export function useSetNoPlateNote() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({ cell, note }: { cell: PlanCell; note: string | null }) =>
      setAvailabilityOverride(cell.date, cell.slot_id, { mode: "none", note }),
    onSuccess: () => {
      void invalidatePlanViews(qc)
      void qc.invalidateQueries({ queryKey: availabilityKeys.ranges() })
    },
    onError: (err) => toastError(err, t),
  })
}
