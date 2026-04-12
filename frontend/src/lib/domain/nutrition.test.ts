import { describe, it, expect } from "vitest"
import { fromIngredients, perPortion } from "./nutrition"
import type { Macros, IngredientInput } from "./nutrition"
import cases from "./testdata/nutrition-cases.json"

interface TestCase {
  name: string
  ingredients: IngredientInput[]
  reference_portions: number
  expected_total: Macros
  expected_per_portion: Macros
}

const tolerance = 0.01

function assertMacrosClose(got: Macros, want: Macros) {
  expect(got.kcal).toBeCloseTo(want.kcal, tolerance)
  expect(got.protein).toBeCloseTo(want.protein, tolerance)
  expect(got.fat).toBeCloseTo(want.fat, tolerance)
  expect(got.carbs).toBeCloseTo(want.carbs, tolerance)
  expect(got.fiber).toBeCloseTo(want.fiber, tolerance)
  expect(got.sodium).toBeCloseTo(want.sodium, tolerance)
}

describe("fromIngredients", () => {
  for (const tc of cases as TestCase[]) {
    it(tc.name, () => {
      const got = fromIngredients(tc.ingredients)
      assertMacrosClose(got, tc.expected_total)
    })
  }
})

describe("perPortion", () => {
  for (const tc of cases as TestCase[]) {
    it(tc.name, () => {
      const got = perPortion({
        ingredients: tc.ingredients,
        reference_portions: tc.reference_portions,
      })
      assertMacrosClose(got, tc.expected_per_portion)
    })
  }
})
