import { beforeEach, describe, expect, it } from "vitest"

import type { Plan } from "@/lib/api/plan"
import i18n from "@/lib/i18n"
import {
  mockPlan,
  mockPlate,
  mockPlateComponent,
  mockSession,
} from "@/test/fixtures"

import { buildChips } from "./chips"

const on = { freezing: true, freezable: true }
const texts = (plan: Plan, opts = on) =>
  buildChips(plan, opts, i18n).map((c) => c.text)

const draftSession = { ...mockSession, state: "draft" as const }

/** A draft plate with a side on Sunday 1 Nov. */
function withDraftSide(plan: Plan): Plan {
  return {
    ...plan,
    cells: plan.cells.map((c) =>
      c.date === "2026-11-01" && c.slot_id === 3
        ? {
            ...c,
            plate: mockPlate({
              id: 50,
              date: c.date,
              slot_id: 3,
              state: "draft",
              components: [
                mockPlateComponent({ id: 1, role: "main" }),
                mockPlateComponent({
                  id: 2,
                  role: "side_starch",
                  food_name: "Rice",
                }),
              ],
            }),
          }
        : { ...c, plate: null }
    ),
  }
}

beforeEach(async () => {
  await i18n.changeLanguage("en")
})

describe("buildChips", () => {
  it("always offers the two fixed chips", () => {
    expect(texts({ ...mockPlan, sessions: [] })).toEqual([
      "Lighter dinners",
      "No fish this week",
    ])
  })

  it("adds the freeze chip for a draft session of a freezable recipe", () => {
    const plan = { ...mockPlan, sessions: [draftSession] }
    expect(texts(plan)[0]).toBe("Make the Chili 8 portions, freeze 2")
  })

  it("leaves the freeze chip out when freezing is off, the recipe is not freezable or the session is confirmed", () => {
    const plan = { ...mockPlan, sessions: [draftSession] }
    expect(texts(plan, { freezing: false, freezable: true })).toHaveLength(2)
    expect(texts(plan, { freezing: true, freezable: false })).toHaveLength(2)
    expect(texts({ ...mockPlan, sessions: [mockSession] })).toHaveLength(2)
  })

  it("adds a different-side chip for the first draft plate with a side", () => {
    expect(texts(withDraftSide(mockPlan))[0]).toBe("Different side on Sunday")
  })

  it("ignores sides on confirmed plates", () => {
    const plan = withDraftSide(mockPlan)
    plan.cells = plan.cells.map((c) =>
      c.plate ? { ...c, plate: { ...c.plate, state: "confirmed" } } : c
    )
    expect(texts({ ...plan, sessions: [] })).toEqual([
      "Lighter dinners",
      "No fish this week",
    ])
  })

  it("speaks German", async () => {
    await i18n.changeLanguage("de")
    expect(texts({ ...withDraftSide(mockPlan), sessions: [] })).toEqual([
      "Andere Beilage am Sonntag",
      "Leichtere Abendessen",
      "Diese Woche kein Fisch",
    ])
  })
})
