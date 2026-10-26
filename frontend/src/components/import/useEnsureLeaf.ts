import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { ApiError } from "@/lib/api/client"
import { createFood, type LeafFoodInput } from "@/lib/api/foods"
import type { LookupCandidate } from "@/lib/api/lookup"
import { foodKeys } from "@/lib/queries/keys"
import { toastError } from "@/lib/toast"

export interface EnsuredLeaf {
  id: number
  name: string
}

/** Leaf payload for an external (FDC/OFF) lookup candidate. Missing per-100 g
 *  values stay null — never 0 — so nutrition completeness can flag them. */
export function leafInputFromCandidate(c: LookupCandidate): LeafFoodInput {
  return {
    kind: "leaf",
    name: c.name,
    source: c.source,
    barcode: c.barcode,
    off_id: c.source === "off" ? c.barcode : null,
    fdc_id: c.fdc_id != null ? String(c.fdc_id) : null,
    kcal_100g: c.kcal_100g,
    protein_100g: c.protein_100g,
    fat_100g: c.fat_100g,
    carbs_100g: c.carbs_100g,
    fiber_100g: c.fiber_100g,
    sodium_100g: c.sodium_100g,
    saturated_fat_100g: c.saturated_fat_100g ?? null,
    trans_fat_100g: c.trans_fat_100g ?? null,
    cholesterol_100g: c.cholesterol_100g ?? null,
    sugar_100g: c.sugar_100g ?? null,
    potassium_100g: c.potassium_100g ?? null,
    calcium_100g: c.calcium_100g ?? null,
    iron_100g: c.iron_100g ?? null,
    magnesium_100g: c.magnesium_100g ?? null,
    phosphorus_100g: c.phosphorus_100g ?? null,
    zinc_100g: c.zinc_100g ?? null,
    vitamin_a_100g: c.vitamin_a_100g ?? null,
    vitamin_c_100g: c.vitamin_c_100g ?? null,
    vitamin_d_100g: c.vitamin_d_100g ?? null,
    vitamin_b12_100g: c.vitamin_b12_100g ?? null,
    vitamin_b6_100g: c.vitamin_b6_100g ?? null,
    folate_100g: c.folate_100g ?? null,
  }
}

/** Creates the leaf, or — when a leaf with that name already exists (409) —
 *  returns the existing one instead. */
async function createLeafOrReuse(input: LeafFoodInput): Promise<EnsuredLeaf> {
  try {
    const food = await createFood(input)
    return { id: food.id, name: food.name }
  } catch (err) {
    if (err instanceof ApiError && err.status === 409 && err.existingId) {
      return { id: err.existingId, name: input.name }
    }
    throw err
  }
}

export function useEnsureLeaf() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createLeafOrReuse,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: foodKeys.lists() })
    },
    onError: (err) => toastError(err, t),
  })
}
