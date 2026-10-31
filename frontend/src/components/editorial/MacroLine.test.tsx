import { render, screen } from "@testing-library/react"
import { beforeEach, describe, expect, it } from "vitest"

import i18n from "@/lib/i18n"

import { MacroLine } from "./MacroLine"

beforeEach(async () => {
  await i18n.changeLanguage("en")
})

describe("MacroLine", () => {
  it("reads kcal, protein, fat and carbs as one line of text", () => {
    render(<MacroLine kcal={452.4} protein={31} fat={12.25} carbs={50} />)
    expect(screen.getByTestId("nutrition-line")).toHaveTextContent(
      "452 kcal · P 31 g · F 12.3 g · C 50 g"
    )
  })

  it("uses the German abbreviations and number format", async () => {
    await i18n.changeLanguage("de")
    render(<MacroLine kcal={1840} protein={4.5} fat={3} carbs={60} />)
    expect(screen.getByTestId("nutrition-line")).toHaveTextContent(
      "1.840 kcal · E 4,5 g · F 3 g · K 60 g"
    )
  })

  it("shows a dash for a missing value", () => {
    render(<MacroLine kcal={null} protein={10} fat={undefined} carbs={5} />)
    expect(screen.getByTestId("nutrition-line")).toHaveTextContent(
      "— kcal · P 10 g · F — g · C 5 g"
    )
  })

  it("sets tabular numerals in Cookbook ink, without a macro palette", () => {
    render(<MacroLine kcal={100} protein={1} fat={1} carbs={1} />)
    const line = screen.getByTestId("nutrition-line")
    expect(line.className).toMatch(/tabular-nums/)
    expect(line.className).toMatch(/text-ink-muted/)
    expect(line.innerHTML).not.toMatch(/protein|carbs|chart/)
  })
})
