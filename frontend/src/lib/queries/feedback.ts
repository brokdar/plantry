import { useMutation, useQueryClient } from "@tanstack/react-query"

import type { PlateFeedback, PutFeedbackInput } from "@/lib/api/feedback"
import { deleteFeedback, putFeedback } from "@/lib/api/feedback"
import type { Plate } from "@/lib/api/plates"

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
  })
}

export function useClearFeedback() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (plateId: number) => deleteFeedback(plateId),
    onSuccess: (_, plateId) => {
      patchFeedbackInCache(qc, plateId, null)
    },
  })
}
