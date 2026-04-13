export interface Macros {
  kcal: number
  protein: number
  fat: number
  carbs: number
  fiber: number
  sodium: number
}

export interface IngredientInput {
  per_100g: Macros
  grams: number
}

export interface ComponentInput {
  ingredients: IngredientInput[]
  reference_portions: number
}

export function fromIngredients(items: IngredientInput[]): Macros {
  const m: Macros = {
    kcal: 0,
    protein: 0,
    fat: 0,
    carbs: 0,
    fiber: 0,
    sodium: 0,
  }
  for (const it of items) {
    const factor = it.grams / 100
    m.kcal += it.per_100g.kcal * factor
    m.protein += it.per_100g.protein * factor
    m.fat += it.per_100g.fat * factor
    m.carbs += it.per_100g.carbs * factor
    m.fiber += it.per_100g.fiber * factor
    m.sodium += it.per_100g.sodium * factor
  }
  return m
}

export function perPortion(c: ComponentInput): Macros {
  const total = fromIngredients(c.ingredients)
  if (c.reference_portions <= 0) return total
  return {
    kcal: total.kcal / c.reference_portions,
    protein: total.protein / c.reference_portions,
    fat: total.fat / c.reference_portions,
    carbs: total.carbs / c.reference_portions,
    fiber: total.fiber / c.reference_portions,
    sodium: total.sodium / c.reference_portions,
  }
}
