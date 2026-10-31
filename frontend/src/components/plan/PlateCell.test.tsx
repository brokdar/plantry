import { cleanup, render, screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import i18n from "@/lib/i18n"
import { buildBoard, type BoardCell } from "@/lib/plan/board"
import { mockPlan, mockSessionWithWarnings } from "@/test/fixtures"

import { PlateCell } from "./PlateCell"

function cell(key: string, plan = mockPlan): BoardCell {
  const found = buildBoard(plan, i18n)
    .days.flatMap((d) => d.cells)
    .find((c) => c.key === key)
  if (!found) throw new Error(key)
  return found
}

function renderCell(c: BoardCell, readOnly = false) {
  render(
    <PlateCell
      cell={c}
      layout="grid"
      readOnly={readOnly}
      dnd={false}
      onOpen={vi.fn()}
    />
  )
  return screen.getByTestId("plan-cell")
}

beforeEach(async () => {
  await i18n.changeLanguage("en")
})

describe("PlateCell", () => {
  it("draws the cook cell on the batch tint with an inset ink outline", () => {
    const button = renderCell(cell("2026-11-01|3"))
    expect(button.className).toMatch(/bg-batch-1-tint/)
    expect(button.className).toMatch(/inset-ring-\[1\.5px\]/)
    expect(button.className).toMatch(/inset-ring-ink/)
    expect(screen.getByText("Cook × 6")).toBeInTheDocument()
    expect(screen.getByText("cook here")).toBeInTheDocument()
  })

  it("keeps a leftover cell on the plain panel with its badge", () => {
    const button = renderCell(cell("2026-11-02|3"))
    expect(button.className).toMatch(/bg-panel/)
    expect(button.className).not.toMatch(/tint/)
    expect(screen.getByText("3/3")).toBeInTheDocument()
    expect(screen.getByText("leftover")).toBeInTheDocument()
  })

  it("shows No plate with its note, and a routine with its name", () => {
    const plan = {
      ...mockPlan,
      cells: mockPlan.cells.map((c) =>
        c.date === "2026-11-03" && c.slot_id === 1
          ? { ...c, mode: "none" as const, note: "Hotel" }
          : c.date === "2026-11-04" && c.slot_id === 1
            ? { ...c, mode: "routine" as const, preset_name: "Oats" }
            : c
      ),
    }
    renderCell(cell("2026-11-03|1", plan))
    expect(screen.getByText("No plate")).toHaveClass("italic")
    expect(screen.getByText("Hotel")).toBeInTheDocument()
    cleanup()
    renderCell(cell("2026-11-04|1", plan))
    expect(screen.getByText("Routine · Oats")).toBeInTheDocument()
  })

  it("offers Add in an empty cell, except in a past window", () => {
    renderCell(cell("2026-11-03|2"))
    expect(screen.getByText("Add")).toBeInTheDocument()
    cleanup()
    renderCell(cell("2026-11-03|2"), true)
    expect(screen.queryByText("Add")).not.toBeInTheDocument()
  })

  it("marks a cell that needs attention", () => {
    const plan = {
      ...mockPlan,
      sessions: [{ ...mockSessionWithWarnings, id: 7 }],
    }
    renderCell(cell("2026-11-02|2", plan))
    expect(screen.getByTestId("cell-warning")).toBeInTheDocument()
  })
})
