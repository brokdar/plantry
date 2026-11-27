import type { Draft, DraftIngredient } from "@/lib/api/import"

import type { RowState } from "./IngredientResolveRow"

/** The catalogue food the backend's match accepted for a line (MATCH-2: it
 *  checked nutrition, state and kcal band); null leaves the line for the
 *  user. Seeded here, not in the row, so a phone's collapsed rows have it
 *  too. */
function acceptedFood(ing: DraftIngredient) {
  const m = ing.match
  const picked =
    m && m.status !== "unmatched" && m.pick >= 0
      ? m.candidates[m.pick]
      : undefined
  return picked?.existing_id
    ? { id: picked.existing_id, name: picked.name }
    : null
}

/** The review's starting rows for a draft: a line starts linked to the
 *  backend's accepted match, otherwise it waits for one (P3: nothing is
 *  skipped; a line is matched or removed). */
export function draftRows(d: Draft): RowState[] {
  return d.ingredients.map((ing) => ({
    resolution: "existing",
    foodId: acceptedFood(ing)?.id ?? null,
    foodName: acceptedFood(ing)?.name ?? null,
    amount: ing.amount,
    unit: ing.unit || "g",
    countUnit: ing.portion_grams !== null || ing.needs_weight ? ing.unit : null,
    portionGrams: ing.portion_grams,
    portionEdited: false,
    toTaste: !!ing.to_taste,
    proposed: !!ing.proposed,
    removed: false,
    query: ing.name || ing.raw_text,
  }))
}

/** What still blocks Save on a row: a food, an amount or a weight per
 *  count unit; null when the row is ready (or removed). */
export type RowProblem = "match" | "amount" | "weight"

export function rowProblem(r: RowState): RowProblem | null {
  if (r.removed) return null
  if (!r.foodId) return "match"
  if (r.toTaste) return null
  if (!(r.amount > 0)) return "amount"
  if (r.unit === r.countUnit && !((r.portionGrams ?? 0) > 0)) return "weight"
  return null
}

function likelyProblem(ing: DraftIngredient): boolean {
  return (
    ing.confidence === "unparsed" ||
    ing.match?.status === "unmatched" ||
    (!ing.to_taste && ing.amount === 0) ||
    ing.needs_weight
  )
}

/** Row indexes with the problem lines first (REV-3), otherwise in recipe
 *  order. Computed once per draft so rows do not jump while being fixed. */
export function draftOrder(d: Draft): number[] {
  const idx = d.ingredients.map((_, i) => i)
  const problems = idx.filter((i) => likelyProblem(d.ingredients[i]))
  return [...problems, ...idx.filter((i) => !problems.includes(i))]
}
