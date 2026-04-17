import { useMutation, useQueryClient } from "@tanstack/react-query"

import {
  deleteFeedback,
  putFeedback,
  type PlateFeedback,
  type PutFeedbackInput,
} from "@/lib/api/feedback"
import type { Week } from "@/lib/api/weeks"

import { weekKeys } from "./keys"

// patchPlateFeedback returns a week with the given plate's feedback replaced.
// Used for optimistic updates so the PlateFeedbackBar flips state without
// waiting for the server round-trip.
function patchPlateFeedback(
  w: Week | undefined,
  plateId: number,
  feedback: PlateFeedback | null
): Week | undefined {
  if (!w) return w
  return {
    ...w,
    plates: w.plates.map((p) => (p.id === plateId ? { ...p, feedback } : p)),
  }
}

export function useRecordFeedback(weekId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      plateId,
      input,
    }: {
      plateId: number
      input: PutFeedbackInput
    }) => putFeedback(plateId, input),
    onMutate: async ({ plateId, input }) => {
      await qc.cancelQueries({ queryKey: weekKeys.byId(weekId) })
      await qc.cancelQueries({ queryKey: weekKeys.current() })
      const previous = {
        byId: qc.getQueryData<Week>(weekKeys.byId(weekId)),
        current: qc.getQueryData<Week>(weekKeys.current()),
      }
      const optimistic: PlateFeedback = {
        plate_id: plateId,
        status: input.status,
        note: input.note ?? null,
        rated_at: new Date().toISOString(),
      }
      qc.setQueryData<Week | undefined>(weekKeys.byId(weekId), (old) =>
        patchPlateFeedback(old, plateId, optimistic)
      )
      qc.setQueryData<Week | undefined>(weekKeys.current(), (old) =>
        old?.id === weekId ? patchPlateFeedback(old, plateId, optimistic) : old
      )
      return previous
    },
    onError: (_err, _vars, ctx) => {
      if (!ctx) return
      qc.setQueryData(weekKeys.byId(weekId), ctx.byId)
      qc.setQueryData(weekKeys.current(), ctx.current)
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: weekKeys.all })
    },
  })
}

export function useClearFeedback(weekId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (plateId: number) => deleteFeedback(plateId),
    onMutate: async (plateId) => {
      await qc.cancelQueries({ queryKey: weekKeys.byId(weekId) })
      await qc.cancelQueries({ queryKey: weekKeys.current() })
      const previous = {
        byId: qc.getQueryData<Week>(weekKeys.byId(weekId)),
        current: qc.getQueryData<Week>(weekKeys.current()),
      }
      qc.setQueryData<Week | undefined>(weekKeys.byId(weekId), (old) =>
        patchPlateFeedback(old, plateId, null)
      )
      qc.setQueryData<Week | undefined>(weekKeys.current(), (old) =>
        old?.id === weekId ? patchPlateFeedback(old, plateId, null) : old
      )
      return previous
    },
    onError: (_err, _vars, ctx) => {
      if (!ctx) return
      qc.setQueryData(weekKeys.byId(weekId), ctx.byId)
      qc.setQueryData(weekKeys.current(), ctx.current)
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: weekKeys.all })
    },
  })
}
