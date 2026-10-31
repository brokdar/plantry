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
    expect(items).toHaveLength(3)
    expect(items[0]).toHaveAccessibleName("Sat · Pasta × 2")
    expect(items[1]).toHaveAccessibleName("Sun · Chili × 6")
    expect(items[2]).toHaveAccessibleName(
      "Wed · Curry × 8, 2 spare, 2 to freeze"
    )
  })

  it.each([
    // 6 cooked, 2 for the freezer, 5 eaten (none frozen): only 1 is left,
    // and it is the freezer's.
    [
      "a surplus smaller than the planned freezer portions",
      { portions: 6, frozen_portions: 2, consumed: 5, surplus: 1 },
      "Wed · Curry × 6, 1 to freeze",
    ],
    // Eaters outrun the pot: nothing is left to freeze or spare.
    [
      "a deficit",
      { portions: 4, frozen_portions: 2, consumed: 6, surplus: 0 },
      "Wed · Curry × 4",
    ],
    [
      "a plain surplus",
      { portions: 8, frozen_portions: 0, consumed: 6, surplus: 2 },
      "Wed · Curry × 8, 2 spare",
    ],
  ])("splits %s into spare and to-freeze", (_, patch, name) => {
    render(
      <CookStrip
        batches={colourBatches([{ ...curry, ...patch }], mockPlan.slots)}
      />
    )
    expect(screen.getByRole("button", { name })).toBeInTheDocument()
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
    expect(screen.getByRole("button")).toHaveAccessibleName(
      /^So\.? · Chili × 6$/
    )
    expect(screen.getByRole("region", { name: "Kochtage" })).toBeInTheDocument()
  })

  it("truncates a long name but keeps the portions, with the name in title", () => {
    const name = "Slow-cooked smoky black bean and sweet potato chili"
    render(
      <CookStrip
        batches={colourBatches([{ ...curry, food_name: name }], mockPlan.slots)}
      />
    )
    const label = screen.getByTitle(name)
    expect(label).toHaveTextContent(name)
    expect(label).toHaveClass("truncate")
    expect(label.closest("[data-batch-badge]")).toHaveTextContent(
      `Wed · ${name} × 8`
    )
  })

  it("renders nothing without batches", () => {
    const { container } = render(<CookStrip batches={[]} />)
    expect(container).toBeEmptyDOMElement()
  })
})
