import { fireEvent, render, screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import i18n from "@/lib/i18n"
import { buildBoard } from "@/lib/plan/board"
import { mockPlan } from "@/test/fixtures"

import { BatchHighlightProvider } from "./BatchHighlightProvider"
import { BatchLanes } from "./BatchLanes"

function renderLanes() {
  const board = buildBoard(mockPlan, i18n)
  render(
    <BatchHighlightProvider>
      <BatchLanes
        batches={board.cookStrip}
        days={board.days}
        columns="grid-cols-[6rem_repeat(7,1fr)]"
      />
    </BatchHighlightProvider>
  )
  return board
}

describe("BatchLanes", () => {
  it("is a labelled list with one button per batch", () => {
    renderLanes()
    const list = within(
      screen.getByRole("region", { name: "Cook days" })
    ).getByRole("list")
    const buttons = within(list).getAllByRole("button")
    expect(buttons).toHaveLength(1)
    expect(buttons[0]).toHaveAccessibleName("Sun · Chili × 6")
  })

  it("draws a filled dot on the cook day and hollow dots on the leftovers", () => {
    renderLanes()
    const dots = screen
      .getByRole("button", { name: /Chili × 6/ })
      .querySelectorAll("[style*='grid-column']")
    // The line wrapper plus one dot per day the batch is eaten (Sun, Mon, Mon).
    const filled = [...dots].filter((d) => d.className.includes("size-3"))
    expect(filled).toHaveLength(1)
  })

  it("dims the other lanes while one is pointed at", () => {
    renderLanes()
    const button = screen.getByRole("button", { name: /Chili × 6/ })
    fireEvent.pointerEnter(button)
    expect(button.closest("li")).toHaveAttribute("data-highlighted", "1")
    expect(screen.getByRole("list")).toHaveAttribute("data-highlight-active")
  })
})
