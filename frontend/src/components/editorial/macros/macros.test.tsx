import { render, screen } from "@testing-library/react"
import { describe, it, expect, beforeEach } from "vitest"
import i18n from "@/lib/i18n"

import { MacroChip } from "./MacroChip"
import { MacroTriad } from "./MacroTriad"
import { MacroDistributionBar } from "./MacroDistributionBar"
import { MacroKcalHero } from "./MacroKcalHero"

beforeEach(async () => {
  await i18n.changeLanguage("en")
})

describe("MacroChip", () => {
  it("renders full label by default", () => {
    render(<MacroChip kind="protein" grams={1.2} />)
    expect(screen.getByText("Protein")).toBeInTheDocument()
    expect(screen.getByText(/1\.2/)).toBeInTheDocument()
  })

  it("renders abbreviation when abbreviated", () => {
    render(<MacroChip kind="carbs" grams={30} abbreviated />)
    expect(screen.getByText("C")).toBeInTheDocument()
  })

  it("renders localized abbreviation in German", async () => {
    await i18n.changeLanguage("de")
    render(<MacroChip kind="carbs" grams={30} abbreviated />)
    expect(screen.getByText("K")).toBeInTheDocument()
  })

  it("renders em dash for null value", () => {
    render(<MacroChip kind="fat" grams={null} />)
    expect(screen.getByText("—")).toBeInTheDocument()
  })
})

describe("MacroTriad", () => {
  it("renders three chips in protein/carbs/fat order", () => {
    render(<MacroTriad values={{ protein: 10, carbs: 20, fat: 5 }} size="sm" />)
    expect(screen.getByTestId("macro-chip-protein")).toBeInTheDocument()
    expect(screen.getByTestId("macro-chip-carbs")).toBeInTheDocument()
    expect(screen.getByTestId("macro-chip-fat")).toBeInTheDocument()
  })
})

describe("MacroDistributionBar", () => {
  it("renders segments weighted by kcal contribution", () => {
    const { container } = render(
      <MacroDistributionBar values={{ protein: 10, carbs: 10, fat: 10 }} />
    )
    const segments = container.querySelectorAll("[data-macro]")
    expect(segments.length).toBe(3)
    // fat @ 9 kcal/g dominates over protein/carbs @ 4 kcal/g
    const fat = container.querySelector('[data-macro="fat"]') as HTMLElement
    expect(fat.style.width).toMatch(/5[23]\./)
  })

  it("skips zero-width segments", () => {
    const { container } = render(
      <MacroDistributionBar values={{ protein: 10, carbs: 0, fat: 0 }} />
    )
    expect(container.querySelectorAll("[data-macro]").length).toBe(1)
  })
})

describe("MacroKcalHero", () => {
  it("rounds kcal and shows unit", () => {
    render(<MacroKcalHero kcal={123.6} />)
    expect(screen.getByText("124")).toBeInTheDocument()
    expect(screen.getByText("kcal")).toBeInTheDocument()
  })
})
