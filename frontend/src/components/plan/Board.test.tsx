import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import i18n from "@/lib/i18n"
import { buildBoard } from "@/lib/plan/board"
import { mockPlan } from "@/test/fixtures"
import { createHookWrapper } from "@/test/render"

import { Board } from "./Board"
import type { BoardLayoutProps } from "./BoardGrid"
import { DayList } from "./DayList"

/** `Board` renders the grid where there is no phone media query (jsdom),
 *  with the real sensors and drop rules. */
function renderGrid(p: BoardLayoutProps) {
  return render(<Board {...p} />, { wrapper: createHookWrapper() })
}

function props(overrides: Partial<BoardLayoutProps> = {}): BoardLayoutProps {
  return {
    board: buildBoard(mockPlan, i18n),
    targets: mockPlan.targets,
    label: "Meal plan",
    onOpenCell: vi.fn(),
    onMarkNoPlate: vi.fn(),
    ...overrides,
  }
}

/** The plates and badges a layout shows, as (cell name, badge text). */
function platesShown(container: HTMLElement) {
  return [...container.querySelectorAll("[data-cell-key]")]
    .filter((cell) => cell.querySelector("[data-batch-badge]"))
    .map((cell) => [
      cell.getAttribute("data-cell-key"),
      cell.querySelector("[data-batch-badge]")?.textContent,
    ])
    .sort()
}

beforeEach(async () => {
  await i18n.changeLanguage("en")
})

describe("Board layouts", () => {
  it("F3-AC3: the grid and the day list show the same plates and badges", () => {
    const grid = renderGrid(props())
    const gridShown = platesShown(grid.container)
    grid.unmount()

    const list = render(<DayList {...props()} />)
    const listShown = platesShown(list.container)

    expect(gridShown).toEqual([
      ["2026-11-01|3", "Cook × 6"],
      ["2026-11-02|2", "2/3"],
      ["2026-11-02|3", "3/3"],
    ])
    expect(listShown).toEqual(gridShown)
  })

  it("renders a real grid with column and row headers", () => {
    renderGrid(props())
    const grid = screen.getByRole("grid", { name: "Meal plan" })
    expect(within(grid).getAllByRole("columnheader")).toHaveLength(8)
    expect(
      within(grid)
        .getAllByRole("rowheader")
        .map((h) => h.textContent)
    ).toEqual(["Breakfast", "Lunch", "Dinner"])
    expect(
      within(grid).getByRole("button", {
        name: "Monday dinner, Chili, leftover 3 of 3, confirmed",
      })
    ).toBeInTheDocument()
  })

  it("F3-AC4: arrow keys move focus across days and slots, with one tab stop", async () => {
    const user = userEvent.setup()
    renderGrid(props())
    const cells = screen.getAllByTestId("plan-cell")
    const stops = cells.filter((c) => c.tabIndex === 0)
    expect(stops).toHaveLength(1)

    stops[0].focus()
    expect(document.activeElement).toHaveAccessibleName(
      "Saturday breakfast, nothing planned"
    )
    await user.keyboard("{ArrowDown}{ArrowDown}{ArrowRight}")
    expect(document.activeElement).toHaveAccessibleName(
      "Sunday dinner, Chili, cook here, 6 portions, confirmed"
    )
    // Wraps from the end of the breakfast row to the start of lunch.
    await user.keyboard("{ArrowUp}{ArrowUp}{End}{ArrowRight}")
    expect(document.activeElement).toHaveAccessibleName(
      "Saturday lunch, nothing planned"
    )
  })

  it("Enter opens the cell and N marks it No plate", async () => {
    const user = userEvent.setup()
    const p = props()
    renderGrid(p)
    screen
      .getByRole("button", {
        name: "Monday lunch, Chili, leftover 2 of 3, confirmed",
      })
      .focus()
    await user.keyboard("{Enter}")
    expect(p.onOpenCell).toHaveBeenCalledWith(
      expect.objectContaining({ key: "2026-11-02|2" })
    )
    await user.keyboard("n")
    expect(p.onMarkNoPlate).toHaveBeenCalledWith(
      expect.objectContaining({ key: "2026-11-02|2" })
    )
  })

  it("does not mark No plate in a past window", async () => {
    const user = userEvent.setup()
    const board = buildBoard(
      { ...mockPlan, window: { ...mockPlan.window, today: "2026-12-01" } },
      i18n
    )
    const p = props({ board })
    renderGrid(p)
    screen.getAllByTestId("plan-cell")[0].focus()
    await user.keyboard("n")
    expect(p.onMarkNoPlate).not.toHaveBeenCalled()
    expect(screen.queryByText("Add")).not.toBeInTheDocument()
  })

  it("collapses an all-routine row to one line in both layouts", () => {
    const plan = {
      ...mockPlan,
      cells: mockPlan.cells.map((c) =>
        c.slot_id === 1
          ? { ...c, mode: "routine" as const, preset_name: "Oats" }
          : c
      ),
    }
    const board = buildBoard(plan, i18n)
    const grid = renderGrid(props({ board }))
    expect(screen.getByText("Breakfast: Oats every day")).toBeInTheDocument()
    grid.unmount()
    render(<DayList {...props({ board })} />)
    expect(screen.getByText("Breakfast: Oats every day")).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: /breakfast/ })
    ).not.toBeInTheDocument()
  })
})
