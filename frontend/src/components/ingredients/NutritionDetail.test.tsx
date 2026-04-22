import { describe, expect, test } from "vitest"
import { render, screen } from "@testing-library/react"

import { NutritionDetail } from "./NutritionDetail"
import type { LookupCandidate } from "@/lib/api/lookup"

function baseCandidate(
  overrides: Partial<LookupCandidate> = {}
): LookupCandidate {
  return {
    name: "Test",
    source: "fdc",
    barcode: null,
    fdc_id: null,
    image_url: null,
    existing_id: null,
    kcal_100g: null,
    protein_100g: null,
    fat_100g: null,
    carbs_100g: null,
    fiber_100g: null,
    sodium_100g: null,
    portions: [],
    ...overrides,
  }
}

describe("NutritionDetail", () => {
  test("renders nothing when all nutrient fields are null", () => {
    const { container } = render(
      <NutritionDetail candidate={baseCandidate()} />
    )
    expect(container.firstChild).toBeNull()
  })

  test("renders only sections that have at least one value", () => {
    render(
      <NutritionDetail
        candidate={baseCandidate({
          kcal_100g: 200,
          protein_100g: 10,
          // no extended macros → section hidden
          // no minerals → section hidden
          vitamin_c_100g: 15,
        })}
      />
    )

    expect(screen.getByText("nutrition.section_core")).toBeInTheDocument()
    expect(
      screen.queryByText("nutrition.section_extended")
    ).not.toBeInTheDocument()
    expect(
      screen.queryByText("nutrition.section_minerals")
    ).not.toBeInTheDocument()
    expect(screen.getByText("nutrition.section_vitamins")).toBeInTheDocument()
  })

  test("converts sodium from g to mg for display", () => {
    render(
      <NutritionDetail
        candidate={baseCandidate({
          sodium_100g: 0.5, // 500 mg
        })}
      />
    )
    expect(screen.getByText("500 mg")).toBeInTheDocument()
  })
})
