import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { revertFoodChange, type FoodEventUndo } from "@/lib/api/foods"
import { applySubstitution, getSubstitutes } from "@/lib/api/substitutions"
import { toastError } from "@/lib/toast"

import { foodKeys, sessionKeys } from "./keys"
import { invalidatePlanViews, useUndoToast } from "./plan"

/** The substitutes for ingredient `foodId`, with the amounts the batches
 *  need. Rule-based and instant: no network beyond our own server. */
export function useSubstitutes(
  foodId: number,
  sessionIds: readonly number[],
  enabled = true
) {
  return useQuery({
    queryKey: ["substitutes", foodId, [...sessionIds]] as const,
    queryFn: () => getSubstitutes(foodId, sessionIds),
    enabled: enabled && foodId > 0 && sessionIds.length > 0,
  })
}

interface ApplyInput {
  sessionIds: readonly number[]
  fromId: number
  toId: number
  /** What the Undo toast says. */
  message: string
}

/** Swaps an ingredient in every listed batch (one token, from the first
 *  event to the last), then offers Undo. */
export function useApplySubstitution() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const offerUndo = useUndoToast()
  const refresh = () =>
    Promise.all([
      invalidatePlanViews(qc),
      qc.invalidateQueries({ queryKey: sessionKeys.all }),
      qc.invalidateQueries({ queryKey: foodKeys.details() }),
    ])
  const revert = useMutation({
    mutationFn: (undo: FoodEventUndo) => revertFoodChange(undo),
    onSuccess: refresh,
    onError: (err) => toastError(err, t),
  })
  return useMutation({
    mutationFn: async ({ sessionIds, fromId, toId }: ApplyInput) => {
      const tokens: FoodEventUndo[] = []
      for (const id of sessionIds) {
        tokens.push((await applySubstitution(id, fromId, toId)).undo)
      }
      return {
        first_event_id: tokens[0].first_event_id,
        last_event_id: tokens[tokens.length - 1].last_event_id,
      }
    },
    onSuccess: async (undo, { message }) => {
      await refresh()
      offerUndo(message, () => revert.mutate(undo))
    },
    onError: (err) => toastError(err, t),
  })
}
