import type { Draft, Resolution } from "@/lib/api/import"

import type { RowState } from "./IngredientResolveRow"

/** The review's starting rows for a draft: unparsed lines skipped, the rest
 *  waiting for a match. */
export function draftRows(d: Draft): RowState[] {
  return d.ingredients.map((ing) => ({
    resolution:
      ing.confidence === "unparsed" ? "skip" : ("existing" as Resolution),
    foodId: null,
    foodName: null,
    amount: ing.amount,
    unit: ing.unit || "g",
    countUnit: ing.portion_grams !== null ? ing.unit : null,
    portionGrams: ing.portion_grams,
    portionEdited: false,
  }))
}
