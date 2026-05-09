import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it } from "vitest"

import {
  mockBrownRice,
  mockChickenBreast,
  mockChickenCurry,
} from "@/test/fixtures"
import type { MacrosResponse, PlateComponent } from "@/lib/api/plates"
import { renderWithRouter } from "@/test/render"

import { DraftPlatePreview } from "./DraftPlatePreview"
import type { TrayItem } from "./ComponentTraySheet"

const noMacros = new Map<number, MacrosResponse>()

describe("DraftPlatePreview", () => {
  it("renders the empty state when nothing is staged or existing", async () => {
    renderWithRouter(
      <DraftPlatePreview items={[]} macrosByFood={noMacros} existing={[]} />
    )
    expect(
      await screen.findByTestId("draft-plate-preview-empty")
    ).toBeInTheDocument()
    expect(screen.queryByTestId("tray-running-kcal")).not.toBeInTheDocument()
  })

  it("renders the running-total kcal scaled by quantity", async () => {
    // 1 portion of curry: per-portion kcal in the macros map is 600.
    const items: TrayItem[] = [
      {
        food: mockChickenCurry,
        quantity: { kind: "composed", portions: 2 },
      },
    ]
    const macros = new Map<number, MacrosResponse>([
      [
        mockChickenCurry.id,
        { kcal: 600, protein: 40, fat: 20, carbs: 50, fiber: 0, sodium: 0 },
      ],
    ])
    renderWithRouter(
      <DraftPlatePreview items={items} macrosByFood={macros} existing={[]} />
    )
    const kcal = await screen.findByTestId("tray-running-kcal")
    // 600 kcal/portion × 2 portions = 1200 kcal.
    expect(kcal.textContent).toMatch(/1200/)
    // Staged pill is visible with the staged-state marker.
    expect(
      screen.getByTestId(`draft-plate-preview-staged-${mockChickenCurry.id}`)
    ).toHaveAttribute("data-state", "staged")
  })

  it("shows existing components when `existing` is provided", async () => {
    const existing: { pc: PlateComponent; food: typeof mockBrownRice }[] = [
      {
        pc: {
          id: 11,
          plate_id: 1,
          food_id: mockBrownRice.id,
          amount: 200,
          unit: "g",
          grams: 200,
          sort_order: 0,
        },
        food: mockBrownRice,
      },
    ]
    const items: TrayItem[] = [
      {
        food: mockChickenBreast,
        quantity: { kind: "leaf", amount: 100, unit: "g" },
      },
    ]
    renderWithRouter(
      <DraftPlatePreview
        items={items}
        macrosByFood={noMacros}
        existing={existing}
      />
    )
    // Existing pill renders with the existing-state marker.
    const ex = await screen.findByTestId(`draft-plate-preview-existing-11`)
    expect(ex).toHaveAttribute("data-state", "existing")
    // Staged pill renders alongside it.
    expect(
      screen.getByTestId(`draft-plate-preview-staged-${mockChickenBreast.id}`)
    ).toBeInTheDocument()
    // Hero is rendered (collage of existing + staged).
    expect(screen.getByTestId("draft-plate-preview-hero")).toBeInTheDocument()
  })

  it("collapses to a single-line summary on mobile, expands on tap", async () => {
    const items: TrayItem[] = [
      {
        food: mockChickenCurry,
        quantity: { kind: "composed", portions: 1 },
      },
    ]
    const macros = new Map<number, MacrosResponse>([
      [
        mockChickenCurry.id,
        { kcal: 600, protein: 40, fat: 20, carbs: 50, fiber: 0, sodium: 0 },
      ],
    ])
    renderWithRouter(
      <DraftPlatePreview
        items={items}
        macrosByFood={macros}
        existing={[]}
        collapsible
      />
    )

    // Default collapsed: the trigger renders, the full preview does not.
    const trigger = await screen.findByTestId("draft-plate-preview-collapsed")
    expect(trigger).toBeInTheDocument()
    expect(screen.queryByTestId("draft-plate-preview")).not.toBeInTheDocument()
    // Even collapsed, kcal is still surfaced so the user doesn't lose the
    // running total just because they haven't tapped to expand.
    expect(screen.getByTestId("tray-running-kcal").textContent).toMatch(/600/)

    const user = userEvent.setup()
    await user.click(trigger)
    expect(await screen.findByTestId("draft-plate-preview")).toBeInTheDocument()
    expect(
      screen.queryByTestId("draft-plate-preview-collapsed")
    ).not.toBeInTheDocument()
  })
})
