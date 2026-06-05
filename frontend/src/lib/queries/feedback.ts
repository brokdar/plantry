import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import type { PlateFeedback, PutFeedbackInput } from "@/lib/api/feedback"
import { deleteFeedback, putFeedback } from "@/lib/api/feedback"
import type { Plate } from "@/lib/api/plates"

import { toastError } from "@/lib/toast"

import { plateKeys } from "./keys"

/**
 * Patch feedback on the plate in every cached plates-range query.
 * The backend range endpoint omits feedback to keep response size small, so
 * we cannot rely on a refetch to reflect the new status — we update in-place.
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
    onSuccess: (feedback, { plateId }) => {
      patchFeedbackInCache(qc, plateId, feedback)
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
    },
    onError: (err) => toastError(err, t),
  })
}
