import { useQueries } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { NutritionCompletenessWarning } from "@/components/components/NutritionCompletenessWarning"
import { getFoodNutrition, type MissingNutritionLeaf } from "@/lib/api/foods"
import { perPortion } from "@/lib/domain/nutrition"
import { resolveGrams } from "@/lib/domain/units"
import { foodKeys } from "@/lib/queries/keys"

import type { RowState } from "./IngredientResolveRow"

interface ImportNutritionSummaryProps {
  rows: readonly RowState[]
  /** False while any row still needs a choice; the total would under-count. */
  complete: boolean
  referencePortions: number
  sourceKcalPerServing: number | null
}

/** Grams of a row as it will be saved: a count unit goes through its
 *  grams per unit, anything else through the shared unit defaults. */
function rowGrams(row: RowState): number {
  if (row.countUnit !== null && row.unit === row.countUnit) {
    return row.amount * (row.portionGrams ?? 0)
  }
  return resolveGrams(row.amount, row.unit).grams
}

/** kcal per portion of the recipe as it will be saved: each linked leaf's
 *  per-100 g nutrition (GET /api/foods/:id/nutrition) scaled by the row's
 *  grams, through the shared `perPortion` helper. */
export function ImportNutritionSummary({
  rows,
  complete,
  referencePortions,
  sourceKcalPerServing,
}: ImportNutritionSummaryProps) {
  const { t, i18n } = useTranslation()
  const linked = rows.filter(
    (r): r is RowState & { foodId: number } =>
      r.resolution === "existing" && r.foodId != null
  )
  const ids = [...new Set(linked.map((r) => r.foodId))]

  const nutrition = useQueries({
    queries: ids.map((id) => ({
      queryKey: foodKeys.nutrition(id),
      queryFn: () => getFoodNutrition(id),
    })),
    combine: (results) => {
      if (results.some((r) => !r.data)) return null
      return new Map(ids.map((id, i) => [id, results[i].data!]))
    },
  })

  const whole = new Intl.NumberFormat(i18n.language, {
    maximumFractionDigits: 0,
  })

  let kcal: number | null = null
  let missing: MissingNutritionLeaf[] = []
  if (complete && nutrition && linked.length > 0) {
    kcal = perPortion({
      ingredients: linked.map((r) => ({
        per_100g: nutrition.get(r.foodId)!,
        grams: rowGrams(r),
      })),
      reference_portions: referencePortions,
    }).kcal
    const byId = new Map<number, MissingNutritionLeaf>()
    for (const n of nutrition.values()) {
      for (const leaf of n.missing) byId.set(leaf.id, leaf)
    }
    missing = [...byId.values()]
  }

  return (
    <div className="flex flex-col gap-3">
      <div
        data-testid="import-nutrition"
        className="flex flex-wrap items-baseline gap-x-6 gap-y-1"
      >
        {kcal != null ? (
          <p className="text-base font-semibold text-ink tabular-nums">
            {t("import.review.kcal_per_portion", { kcal: whole.format(kcal) })}
          </p>
        ) : (
          <p className="text-sm text-ink-muted">
            {t("import.review.kcal_pending")}
          </p>
        )}
        {sourceKcalPerServing != null && (
          <p className="text-sm text-ink-muted tabular-nums">
            {t("import.review.source_kcal", {
              kcal: whole.format(sourceKcalPerServing),
            })}
          </p>
        )}
      </div>
      <NutritionCompletenessWarning missing={missing} openInNewTab />
    </div>
  )
}
