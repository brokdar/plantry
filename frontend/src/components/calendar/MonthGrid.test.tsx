import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { vi, describe, it, expect } from "vitest"

import { renderWithRouter } from "@/test/render"

import { MonthGrid } from "./MonthGrid"

vi.mock("@/lib/api/plates")

// April 2026: starts on Wednesday (dow=3), 30 days → needs 5 weeks with Mon start
describe("MonthGrid", () => {
  it("renders 5 rows × 7 cols for April 2026 with weekStartsOn=1 (Mon)", async () => {
    renderWithRouter(
      <MonthGrid
        year={2026}
        month={3}
        weekStartsOn={1}
        plates={[]}
        search=""
        onCellClick={vi.fn()}
      />
    )

    // April 2026 with Mon start: Apr 1 is Wed, so grid starts Mar 30
    // Last day Apr 30 is Thu → row ends Sun May 3 → 5 rows
    const cells = await screen.findAllByRole("button")
    // 5 rows × 7 cols = 35 cells
    expect(cells).toHaveLength(35)
  })

  it("first column is Sunday when weekStartsOn=0", async () => {
    renderWithRouter(
      <MonthGrid
        year={2026}
        month={3}
        weekStartsOn={0}
        plates={[]}
        search=""
        onCellClick={vi.fn()}
      />
    )

    // Intl.DateTimeFormat weekday:"short" produces "Sun","Mon",… in en locale
    const headers = await screen.findAllByText(
      /^(Sun|Mon|Tue|Wed|Thu|Fri|Sat)$/
    )
    expect(headers[0]).toHaveTextContent("Sun")
  })

  it("first column is Monday when weekStartsOn=1", async () => {
    renderWithRouter(
      <MonthGrid
        year={2026}
        month={3}
        weekStartsOn={1}
        plates={[]}
        search=""
        onCellClick={vi.fn()}
      />
    )

    const headers = await screen.findAllByText(
      /^(Sun|Mon|Tue|Wed|Thu|Fri|Sat)$/
    )
    expect(headers[0]).toHaveTextContent("Mon")
  })

  it("cell click calls onCellClick with the correct ISO date", async () => {
    const onCellClick = vi.fn()
    renderWithRouter(
      <MonthGrid
        year={2026}
        month={3}
        weekStartsOn={1}
        plates={[]}
        search=""
        onCellClick={onCellClick}
      />
    )

    // Wait for async render, then find cell for April 15 via data-date attribute
    await screen.findAllByRole("button")
    const target = document
      .querySelector('[data-date="2026-04-15"]')
      ?.closest("button")
    if (!target) throw new Error("Cell for 2026-04-15 not found")
    await userEvent.click(target)

    expect(onCellClick).toHaveBeenCalledWith("2026-04-15")
  })
})
