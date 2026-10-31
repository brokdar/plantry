import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import type { PutFeedbackInput } from "@/lib/api/feedback"
import { deleteFeedback, putFeedback } from "@/lib/api/feedback"

import { toastError } from "@/lib/toast"

import { foodKeys } from "./keys"
import { invalidatePlanViews } from "./plan"

/** Feedback changes the plan views (Today's rate card) and, through
 *  `not_eaten`, the derived eating history of the food (D16). */
function invalidateFeedback(qc: ReturnType<typeof useQueryClient>) {
  void invalidatePlanViews(qc)
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
    onSuccess: () => {
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
    onSuccess: () => {
      invalidateFeedback(qc)
    },
    onError: (err) => toastError(err, t),
  })
}
