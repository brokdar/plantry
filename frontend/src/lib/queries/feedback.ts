import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import type { PlateFeedback, PutFeedbackInput } from "@/lib/api/feedback"
import { deleteFeedback, putFeedback } from "@/lib/api/feedback"
import type { Plate } from "@/lib/api/plates"

import { toastError } from "@/lib/toast"

import { foodKeys, plateKeys } from "./keys"
import { invalidatePlanViews } from "./plan"

/**
 * Patch feedback on the plate in every cached plates-range query of the old
 * planner. The range endpoint omits feedback, so a refetch would not show the
 * new status. Deleted with the old planner (F4).
 */
function patchFeedbackInCache(
  qc: ReturnType<typeof useQueryClient>,
  plateId: number,
  feedback: PlateFeedback | null
) {
  qc.setQueriesData<{ plates: Plate[] }>({ queryKey: plateKeys.all }, (old) => {
    if (!old?.plates) return old
    return {
      ...old,
      plates: old.plates.map((p) =>
        p.id === plateId ? { ...p, feedback } : p
      ),
    }
  })
}

/** Feedback changes the plan views (Today's rate card) and, through
 *  `not_eaten`, the derived eating history of the food (D16). */
function invalidateFeedback(qc: ReturnType<typeof useQueryClient>) {
  invalidatePlanViews(qc)
  void qc.invalidateQueries({ queryKey: foodKeys.all })
}

export function useRecordFeedback() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({
      plateId,
      input,
    }: {
      plateId: number
      input: PutFeedbackInput
    }) => putFeedback(plateId, input),
    onSuccess: (fb, { plateId }) => {
      patchFeedbackInCache(qc, plateId, {
        status: fb.status,
        note: fb.note ?? null,
        rated_at: fb.rated_at,
      })
      invalidateFeedback(qc)
    },
    onError: (err) => toastError(err, t),
  })
}

export function useClearFeedback() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (plateId: number) => deleteFeedback(plateId),
    onSuccess: (_, plateId) => {
      patchFeedbackInCache(qc, plateId, null)
      invalidateFeedback(qc)
    },
    onError: (err) => toastError(err, t),
  })
}
