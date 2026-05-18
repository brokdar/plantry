import { render } from "@testing-library/react"
import { describe, expect, test } from "vitest"

import "@/lib/i18n"
import type { MacrosResponse } from "@/lib/api/plates"

import { SlotMacroDots } from "./SlotMacroDots"

function macros(kcal: number): MacrosResponse {
  return { kcal, protein: 30, fat: 20, carbs: 70, fiber: 5, sodium: 200 }
}

describe("SlotMacroDots — target indicator", () => {
  test("renders a placeholder when macros are missing", () => {
    // Phase 2 reserves the row's height with an aria-hidden placeholder so
    // the cell doesn't reflow once `usePlateMacros` settles.
    const { getByTestId, queryByTestId } = render(<SlotMacroDots />)
    expect(getByTestId("slot-cell-macros-placeholder")).toBeInTheDocument()
    expect(queryByTestId("slot-cell-kcal")).not.toBeInTheDocument()
  })

  test("hides the dot when no kcalTarget is set", () => {
    const { queryByTestId } = render(<SlotMacroDots macros={macros(700)} />)
    expect(queryByTestId("slot-macros-target-dot")).not.toBeInTheDocument()
  })

  test("marks 'good' inside ±10% of target", () => {
    const { getByTestId } = render(
      <SlotMacroDots macros={macros(720)} kcalTarget={700} />
    )
    expect(getByTestId("slot-macros-target-dot")).toHaveAttribute(
      "data-tone",
      "good"
    )
  })

  test("marks 'near' between 10% and 20% off target", () => {
    const { getByTestId } = render(
      <SlotMacroDots macros={macros(820)} kcalTarget={700} />
    )
    expect(getByTestId("slot-macros-target-dot")).toHaveAttribute(
      "data-tone",
      "near"
    )
  })

  test("marks 'off' beyond ±20%", () => {
    const { getByTestId } = render(
      <SlotMacroDots macros={macros(1000)} kcalTarget={700} />
    )
    expect(getByTestId("slot-macros-target-dot")).toHaveAttribute(
      "data-tone",
      "off"
    )
  })

  test("hides the dot when target is non-positive", () => {
    const { queryByTestId } = render(
      <SlotMacroDots macros={macros(700)} kcalTarget={0} />
    )
    expect(queryByTestId("slot-macros-target-dot")).not.toBeInTheDocument()
  })
})
