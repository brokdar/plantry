import { cleanup, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import i18n from "@/lib/i18n"
import { buildBoard, type BoardCell } from "@/lib/plan/board"
import {
  mockDraftPlan,
  mockPlan,
  mockSessionWithWarnings,
} from "@/test/fixtures"

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

  it("fills its grid row and rises above the sticky meal column when focused", () => {
    const button = renderCell(cell("2026-11-02|3"))
    expect(button).toHaveClass("h-full")
    expect(button.closest("[data-cell-key]")).toHaveClass("focus-within:z-20")
  })

  it("draws a draft with a dashed border and no padlock", () => {
    const button = renderCell(cell("2026-11-02|3", mockDraftPlan))
    expect(button.className).toMatch(/border-dashed/)
    expect(button.className).not.toMatch(/inset-ring-line/)
    expect(screen.queryByTestId("cell-padlock")).not.toBeInTheDocument()
    cleanup()
    // A draft cook cell keeps its tint with a dashed ink outline.
    const cook = renderCell(cell("2026-11-01|3", mockDraftPlan))
    expect(cook.className).toMatch(/bg-batch-1-tint/)
    expect(cook.className).toMatch(/border-dashed border-ink/)
    expect(cook.className).not.toMatch(/inset-ring-ink/)
  })

  it("draws a confirmed plate solid with a padlock", () => {
    const button = renderCell(cell("2026-11-03|3", mockDraftPlan))
    expect(button.className).not.toMatch(/border-dashed/)
    expect(screen.getByTestId("cell-padlock")).toBeInTheDocument()
  })

  it("offers Replace and Different side on a draft plate only", async () => {
    const onReplace = vi.fn()
    const onDifferentSide = vi.fn()
    const sided = {
      ...cell("2026-11-02|3", mockDraftPlan),
      sideComponentId: 77,
    }
    const props = { layout: "grid", dnd: false, onOpen: vi.fn() } as const
    render(
      <PlateCell
        {...props}
        cell={sided}
        readOnly={false}
        onReplace={onReplace}
        onDifferentSide={onDifferentSide}
      />
    )
    await userEvent.click(screen.getByRole("button", { name: "Replace" }))
    await userEvent.click(
      screen.getByRole("button", { name: "Different side" })
    )
    expect(onReplace).toHaveBeenCalledWith(sided)
    expect(onDifferentSide).toHaveBeenCalledWith(sided)
    cleanup()

    // Confirmed, or a past window: none.
    render(
      <PlateCell
        {...props}
        cell={cell("2026-11-03|3", mockDraftPlan)}
        readOnly={false}
        onReplace={onReplace}
        onDifferentSide={onDifferentSide}
      />
    )
    expect(screen.queryByTestId("cell-quick-actions")).not.toBeInTheDocument()
    cleanup()
    render(
      <PlateCell
        {...props}
        cell={sided}
        readOnly
        onReplace={onReplace}
        onDifferentSide={onDifferentSide}
      />
    )
    expect(screen.queryByTestId("cell-quick-actions")).not.toBeInTheDocument()
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
