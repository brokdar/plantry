import { beforeEach, describe, expect, it } from "vitest"

import i18n from "@/lib/i18n"
import { mockShoppingList } from "@/test/fixtures"

import { quantityText, shoppingText } from "./CopyAsText"

const { sections } = mockShoppingList

beforeEach(async () => {
  await i18n.changeLanguage("en")
})

describe("shoppingText", () => {
  it("writes each section header, then one line per item", () => {
    expect(shoppingText(sections, new Set(), i18n)).toBe(
      "Produce\n- Onion, 3 pcs\n\nCans & jars\n- Kidney beans, 600 g"
    )
  })

  it("leaves out ticked items and the sections they empty", () => {
    expect(shoppingText(sections, new Set([51]), i18n)).toBe(
      "Cans & jars\n- Kidney beans, 600 g"
    )
    expect(shoppingText(sections, new Set([50, 51]), i18n)).toBe("")
  })

  it("speaks the active language", async () => {
    await i18n.changeLanguage("de")
    expect(shoppingText(sections, new Set(), i18n)).toBe(
      "Obst & Gemüse\n- Onion, 3 Stk.\n\nKonserven & Gläser\n- Kidney beans, 600 g"
    )
  })
})

describe("quantityText", () => {
  it("prefers the buy quantity over the display quantity", () => {
    const item = {
      ...sections[0].items[0],
      display: { amount: 2.5, unit: "piece" },
      buy: { amount: 3, unit: "piece" },
    }
    expect(quantityText(item, i18n)).toBe("3 pcs")
  })

  it("falls back to the display quantity", () => {
    expect(quantityText(sections[1].items[0], i18n)).toBe("600 g")
  })
})
