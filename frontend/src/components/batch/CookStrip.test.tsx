import { act, fireEvent, render, screen, within } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import i18n from "@/lib/i18n"
import type { Session } from "@/lib/api/sessions"
import { colourBatches } from "@/lib/plan/batch-colours"
import { mockPlan, mockSession } from "@/test/fixtures"

import { useBatchCell } from "./batch-highlight"
import { BatchHighlightProvider } from "./BatchHighlightProvider"
import { CookStrip } from "./CookStrip"

const curry: Session = {
  ...mockSession,
  id: 9,
  food_name: "Curry",
  cook_date: "2026-11-04",
  cook_slot_id: 3,
  portions: 8,
  frozen_portions: 2,
  consumed: 4,
  frozen_consumed: 0,
  // 8 cooked − 4 eaten = 4 left, 2 of them for the freezer.
  surplus: 4,
}
const pasta: Session = {
  ...mockSession,
  id: 10,
  food_name: "Pasta",
  cook_date: "2026-10-31",
  cook_slot_id: 3,
  portions: 2,
  consumed: 2,
  surplus: 0,
}

function CookCell({ id }: { id: number }) {
  return <div data-testid={`cook-${id}`} {...useBatchCell([id], [id])} />
}

afterEach(async () => {
  await act(() => i18n.changeLanguage("en"))
})

describe("CookStrip", () => {
  it("shows one badge per session in cook order", () => {
    const batches = colourBatches([curry, mockSession, pasta], mockPlan.slots)
    render(<CookStrip batches={batches} />)
    const items = within(screen.getByRole("list")).getAllByRole("button")
    expect(items.map((b) => b.textContent)).toEqual([
      "Sat · Pasta × 2",
      "Sun · Chili × 6",
      "Wed · Curry × 82 spare2 to freeze",
    ])
  })

  it("adds dashed spare and to-freeze badges in the batch colour", () => {
    const batches = colourBatches([curry], mockPlan.slots)
    render(<CookStrip batches={batches} />)
    for (const text of ["2 spare", "2 to freeze"]) {
      const badge = screen.getByText(text).closest("[data-batch-badge]")!
      expect(badge).toHaveClass("border-dashed", "border-batch-1")
    }
  })

  it("names its region and scrolls sideways", () => {
    render(<CookStrip batches={colourBatches([mockSession], mockPlan.slots)} />)
    const list = screen.getByRole("list")
    expect(screen.getByRole("region", { name: "Cook days" })).toContainElement(
      list
    )
    expect(list).toHaveClass("overflow-x-auto")
  })

  it("tapping a badge scrolls to the cook cell and highlights the batch", () => {
    const scroll = vi.spyOn(HTMLElement.prototype, "scrollIntoView")
    render(
      <BatchHighlightProvider>
        <CookStrip
          batches={colourBatches([mockSession, curry], mockPlan.slots)}
        />
        <CookCell id={7} />
        <CookCell id={9} />
      </BatchHighlightProvider>
    )
    fireEvent.click(screen.getByRole("button", { name: /Curry/ }))
    expect(scroll.mock.contexts[0]).toBe(screen.getByTestId("cook-9"))
    expect(screen.getByTestId("cook-9")).toHaveAttribute("data-highlighted")
    expect(screen.getByTestId("cook-7")).not.toHaveAttribute("data-highlighted")
    scroll.mockRestore()
  })

  it("reads in German", async () => {
    await act(() => i18n.changeLanguage("de"))
    render(<CookStrip batches={colourBatches([mockSession], mockPlan.slots)} />)
    expect(screen.getByText(/^So\.? · Chili × 6$/)).toBeInTheDocument()
    expect(screen.getByRole("region", { name: "Kochtage" })).toBeInTheDocument()
  })

  it("renders nothing without batches", () => {
    const { container } = render(<CookStrip batches={[]} />)
    expect(container).toBeEmptyDOMElement()
  })
})
