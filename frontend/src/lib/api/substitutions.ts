import type { FoodEventUndo } from "./foods"
import { apiFetch } from "./client"

/** One offered substitute for an ingredient. */
export interface SubstituteOption {
  food_id: number
  name: string
  /** Grams of the substitute per gram of the ingredient. */
  ratio: number
  /** What the asked batches need of it; null for a to-taste line. */
  amount: { amount: number; unit: string } | null
  note_en: string
  note_de: string
}

/** GET /api/substitutes — up to three substitutes for ingredient `foodId`
 *  with the amounts the listed batches need, plus the listed batches the swap
 *  applies to (their recipe has the ingredient and they are not past). */
export function getSubstitutes(
  foodId: number,
  sessionIds: readonly number[]
): Promise<{ options: SubstituteOption[]; session_ids: number[] }> {
  return apiFetch(
    `/substitutes?food_id=${foodId}&session_ids=${sessionIds.join(",")}`
  )
}

/** POST /api/cook-sessions/{id}/overrides — swaps the ingredient for this
 *  batch only; the token undoes it (POST /api/food-events/revert).
 *  `offer_permanent` is true once, the second time the same swap is made in a
 *  recipe; `confirmed_dates` are the confirmed plates of that recipe from
 *  today on (an in-place change shifts their shopping). */
export function applySubstitution(
  sessionId: number,
  fromFoodId: number,
  toFoodId: number
): Promise<{
  undo: FoodEventUndo
  offer_permanent: boolean
  confirmed_dates: string[]
}> {
  return apiFetch(`/cook-sessions/${sessionId}/overrides`, {
    method: "POST",
    body: JSON.stringify({ from_food_id: fromFoodId, to_food_id: toFoodId }),
  })
}

/** POST /api/cook-sessions/{id}/overrides/keep — makes the swap just applied
 *  permanent: in the recipe itself ("recipe") or as a visible variant
 *  ("variant", which also returns the variant's id and name). */
export function keepSubstitution(
  sessionId: number,
  fromFoodId: number,
  toFoodId: number,
  mode: "recipe" | "variant"
): Promise<{ undo: FoodEventUndo; food_id?: number; name?: string }> {
  return apiFetch(`/cook-sessions/${sessionId}/overrides/keep`, {
    method: "POST",
    body: JSON.stringify({
      from_food_id: fromFoodId,
      to_food_id: toFoodId,
      mode,
    }),
  })
}
