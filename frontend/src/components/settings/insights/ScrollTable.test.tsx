import { fireEvent, render, screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"
import "@/lib/i18n"

import { ScrollTable } from "./ScrollTable"

/** A ResizeObserver that reports once on observe, as browsers do. */
class ReportingObserver {
  private cb: () => void
  constructor(cb: () => void) {
    this.cb = cb
  }
  observe() {
    this.cb()
  }
  disconnect() {}
}

function box(scrollWidth: number, clientWidth: number) {
  vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockReturnValue(
    scrollWidth
  )
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(
    clientWidth
  )
}

describe("ScrollTable", () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  // DoD walk nit: at 375 the runs table hid its last columns with no cue.
  it("says the table scrolls while columns are hidden to the right", () => {
    vi.stubGlobal("ResizeObserver", ReportingObserver)
    box(900, 343)
    render(
      <ScrollTable>
        <table />
      </ScrollTable>
    )
    expect(screen.getByTestId("scroll-hint")).toHaveTextContent(
      "Scroll sideways for more columns"
    )

    // Scrolled to the end, nothing is hidden any more.
    const scroller = screen.getByRole("table").parentElement!
    scroller.scrollLeft = 557
    fireEvent.scroll(scroller)
    expect(screen.queryByTestId("scroll-hint")).toBeNull()
  })

  it("shows no hint when the table fits", () => {
    vi.stubGlobal("ResizeObserver", ReportingObserver)
    box(800, 800)
    render(
      <ScrollTable>
        <table />
      </ScrollTable>
    )
    expect(screen.queryByTestId("scroll-hint")).toBeNull()
  })
})
