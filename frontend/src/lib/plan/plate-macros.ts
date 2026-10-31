import type { FoodMacrosEntry } from "@/lib/api/foods"
import type { PlateComponent } from "@/lib/api/plates"

export interface PlateMacros {
  kcal: number
  protein: number
  fat: number
  carbs: number
}

/**
 * A plate's macros for one eater from `GET /api/foods/macros`: composed
 * foods per portion × portions, leaves per 100 g × grams. Eaters do not
 * change it. Foods without an entry count as zero.
 */
export function plateMacros(
  components: readonly PlateComponent[],
  foods: readonly FoodMacrosEntry[]
): PlateMacros {
  const byId = new Map(foods.map((f) => [f.food_id, f.macros]))
  const total = { kcal: 0, protein: 0, fat: 0, carbs: 0 }
  for (const c of components) {
    const m = byId.get(c.food_id)
    if (!m) continue
    const factor =
      c.kind === "composed" ? (c.portions ?? 1) : (c.grams ?? 0) / 100
    total.kcal += m.kcal * factor
    total.protein += m.protein * factor
    total.fat += m.fat * factor
    total.carbs += m.carbs * factor
  }
  return total
}
