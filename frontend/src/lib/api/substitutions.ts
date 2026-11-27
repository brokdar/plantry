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
 *  with the amounts the listed batches need. */
export function getSubstitutes(
  foodId: number,
  sessionIds: readonly number[]
): Promise<{ options: SubstituteOption[] }> {
  return apiFetch(
    `/substitutes?food_id=${foodId}&session_ids=${sessionIds.join(",")}`
  )
}

/** POST /api/cook-sessions/{id}/overrides — swaps the ingredient for this
 *  batch only; the token undoes it (POST /api/food-events/revert). */
export function applySubstitution(
  sessionId: number,
  fromFoodId: number,
  toFoodId: number
): Promise<{ undo: FoodEventUndo }> {
  return apiFetch(`/cook-sessions/${sessionId}/overrides`, {
    method: "POST",
    body: JSON.stringify({ from_food_id: fromFoodId, to_food_id: toFoodId }),
  })
}
