import { act, render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it } from "vitest"

import i18n from "@/lib/i18n"
import type { PlanDay } from "@/lib/api/plan"
import { mockPlan } from "@/test/fixtures"

import { NutritionPopover } from "./NutritionPopover"

const monday: PlanDay = {
  date: "2026-11-02",
  macros: { kcal: 1840.4, protein: 96.2, fat: 58, carbs: 210 },
  complete: true,
  missing: [],
}

async function open(day: PlanDay, targets = mockPlan.targets) {
  const user = userEvent.setup()
  render(
    <NutritionPopover day={day} targets={targets}>
      <button type="button">open</button>
    </NutritionPopover>
  )
  await user.click(screen.getByRole("button", { name: "open" }))
  return screen.findByRole("dialog")
}

function row(dialog: HTMLElement, label: string) {
  return within(dialog).getByText(label).closest("div")!
}

afterEach(async () => {
  await act(() => i18n.changeLanguage("en"))
})

describe("NutritionPopover", () => {
  it("shows kcal, protein, fat and carbs against targets", async () => {
    const dialog = await open(monday)
    expect(dialog).toHaveAccessibleName("Monday, November 2")
    expect(dialog).toHaveTextContent("Per person")
    expect(row(dialog, "Energy")).toHaveTextContent("1,840 / 2,200 kcal")
    expect(row(dialog, "Protein")).toHaveTextContent("96 / 140 g")
    expect(row(dialog, "Fat")).toHaveTextContent("58 / 70 g")
    expect(row(dialog, "Carbs")).toHaveTextContent("210 / 250 g")
    expect(dialog).not.toHaveTextContent("Incomplete")
  })

  it("shows plain amounts without targets", async () => {
    const dialog = await open(monday, null)
    expect(row(dialog, "Energy")).toHaveTextContent(/^Energy1,840 kcal$/)
    expect(row(dialog, "Protein")).toHaveTextContent(/^Protein96 g$/)
  })

  it("explains an incomplete day", async () => {
    const dialog = await open({
      ...monday,
      complete: false,
      missing: ["Kidney beans", "Onion"],
    })
    expect(dialog).toHaveTextContent(
      "IncompleteNo nutrition data for Kidney beans, Onion."
    )
  })

  it("reads in German", async () => {
    await act(() => i18n.changeLanguage("de"))
    const dialog = await open({ ...monday, complete: false, missing: [] })
    expect(dialog).toHaveTextContent("Pro Person")
    expect(row(dialog, "Energie")).toHaveTextContent("1.840 / 2.200 kcal")
    expect(row(dialog, "Eiweiß")).toHaveTextContent("96 / 140 g")
    expect(row(dialog, "Kohlenhydrate")).toHaveTextContent("210 / 250 g")
    expect(dialog).toHaveTextContent("Unvollständig")
  })
})
