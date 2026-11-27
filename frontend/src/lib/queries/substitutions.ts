import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { revertFoodChange, type FoodEventUndo } from "@/lib/api/foods"
import {
  applySubstitution,
  getSubstitutes,
  keepSubstitution,
} from "@/lib/api/substitutions"
import { toast, toastError } from "@/lib/toast"

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

/** One token from the first event to the last of a series of calls. */
function mergedUndo(tokens: readonly FoodEventUndo[]): FoodEventUndo {
  return {
    first_event_id: tokens[0].first_event_id,
    last_event_id: tokens[tokens.length - 1].last_event_id,
  }
}

/** Refreshes everything a swap changes. The dialog calls it when it closes:
 *  refreshing earlier would unmount it (the shop row or cook page it sits in
 *  changes) before the keep choices are made. */
export function useRefreshSwap() {
  const qc = useQueryClient()
  return () =>
    Promise.all([
      invalidatePlanViews(qc),
      qc.invalidateQueries({ queryKey: sessionKeys.all }),
      qc.invalidateQueries({ queryKey: foodKeys.all }),
    ])
}

// The toast of the last swap: keeping the swap replaces it (its Undo range no
// longer reverts cleanly once the recipe or variant changed).
let swapToast: string | number | undefined

/** Offers Undo for a swap; an Undo refreshes at once. */
function useSwapEffects() {
  const { t } = useTranslation()
  const offerUndo = useUndoToast()
  const refresh = useRefreshSwap()
  const revert = useMutation({
    mutationFn: (undo: FoodEventUndo) => revertFoodChange(undo),
    onSuccess: refresh,
    onError: (err) => toastError(err, t),
  })
  return {
    done(undo: FoodEventUndo, message: string, replace = false) {
      if (replace && swapToast !== undefined) toast.dismiss(swapToast)
      swapToast = offerUndo(message, () => revert.mutate(undo))
    },
    onError: (err: Error) => toastError(err, t),
  }
}

/** Swaps an ingredient in every listed batch (one token, from the first
 *  event to the last), then offers Undo. Resolves with the second-time offer
 *  and the confirmed dates of the recipes. The views refresh when the dialog
 *  closes (`useRefreshSwap`). */
export function useApplySubstitution() {
  const fx = useSwapEffects()
  return useMutation({
    mutationFn: async ({ sessionIds, fromId, toId }: ApplyInput) => {
      const tokens: FoodEventUndo[] = []
      let offer = false
      const dates = new Set<string>()
      for (const id of sessionIds) {
        const r = await applySubstitution(id, fromId, toId)
        tokens.push(r.undo)
        offer ||= r.offer_permanent
        r.confirmed_dates.forEach((d) => dates.add(d))
      }
      return { undo: mergedUndo(tokens), offer, dates: [...dates].sort() }
    },
    onSuccess: ({ undo }, { message }) => fx.done(undo, message),
    onError: fx.onError,
  })
}

/** Makes the swap permanent in every listed batch's recipe ("Always in this
 *  recipe") or as a visible variant ("Save as variant"), then offers Undo. */
export function useKeepSubstitution() {
  const fx = useSwapEffects()
  return useMutation({
    mutationFn: async ({
      mode,
      sessionIds,
      fromId,
      toId,
    }: ApplyInput & { mode: "recipe" | "variant" }) => {
      const tokens: FoodEventUndo[] = []
      for (const id of sessionIds) {
        tokens.push((await keepSubstitution(id, fromId, toId, mode)).undo)
      }
      return mergedUndo(tokens)
    },
    onSuccess: (undo, { message }) => fx.done(undo, message, true),
    onError: fx.onError,
  })
}
