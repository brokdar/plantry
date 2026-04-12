import { apiFetch } from "./client"

export interface Portion {
  ingredient_id: number
  unit: string
  grams: number
}

export function listPortions(ingredientId: number): Promise<Portion[]> {
  return apiFetch(`/ingredients/${ingredientId}/portions`)
}

export function upsertPortion(
  ingredientId: number,
  data: { unit: string; grams: number }
): Promise<void> {
  return apiFetch(`/ingredients/${ingredientId}/portions`, {
    method: "POST",
    body: JSON.stringify(data),
  })
}

export function deletePortion(
  ingredientId: number,
  unit: string
): Promise<void> {
  return apiFetch(
    `/ingredients/${ingredientId}/portions/${encodeURIComponent(unit)}`,
    { method: "DELETE" }
  )
}
