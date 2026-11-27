import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import type { PlateFeedbackStatus, PutFeedbackInput } from "@/lib/api/feedback"
import { deleteFeedback, putFeedback } from "@/lib/api/feedback"

import { toast, toastError } from "@/lib/toast"

import { foodKeys } from "./keys"
import { invalidatePlanViews, UNDO_MS } from "./plan"

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

/** Today's rating question: records the answer, then offers Undo for
 *  UNDO_MS, which clears the feedback again. `answered` is the plate whose
 *  answer is pending or saved, so the question can hide at once. */
export function useRatePlate() {
  const { t } = useTranslation()
  const record = useRecordFeedback()
  const clear = useClearFeedback()
  // The plate already sent: mutation state updates a tick late, so a double
  // click would otherwise send the answer twice.
  const sent = useRef<number | null>(null)
  // Trial recipes the rating archived: the reason sheet asks about the first.
  const [archived, setArchived] = useState<{ food_id: number; name: string }[]>(
    []
  )

  function rate(plateId: number, status: PlateFeedbackStatus) {
    if (sent.current === plateId) return
    sent.current = plateId
    record.mutate(
      { plateId, input: { status } },
      {
        onError: () => {
          sent.current = null
        },
        onSuccess: (res) => {
          setArchived(res.archived_trials ?? [])
          toast(t("today.rate.saved"), {
            duration: UNDO_MS,
            action: {
              label: t("common.undo"),
              onClick: () => {
                record.reset()
                sent.current = null
                clear.mutate(plateId)
              },
            },
          })
        },
      }
    )
  }

  return {
    rate,
    archived: archived[0] ?? null,
    dismissArchived: () => setArchived((a) => a.slice(1)),
    answered: record.isError ? null : (record.variables?.plateId ?? null),
  }
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
