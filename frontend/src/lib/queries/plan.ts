import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { clearPlan, getPlan, restorePlan } from "@/lib/api/plan"
import type { PlanSnapshot, RemovedResponse } from "@/lib/api/plates"
import { toast, toastError } from "@/lib/toast"

import { planKeys, shoppingKeys, todayKeys } from "./keys"

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
