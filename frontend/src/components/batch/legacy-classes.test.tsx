import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it } from "vitest"

import "@/lib/i18n"
import { colourBatches } from "@/lib/plan/batch-colours"
import { mockPlan, mockSession } from "@/test/fixtures"

import { BatchBadge } from "./BatchBadge"
import { BatchHighlightProvider } from "./BatchHighlightProvider"
import { CookStrip } from "./CookStrip"
import { DayMeter } from "./DayMeter"

// Plan §0.2 gate 6, the CSS lines: legacy Material aliases and Botanical
// Atelier utilities must not appear in the batch primitives.
const LEGACY =
  /surface-container|on-surface|outline-variant|primary-container|on-primary|secondary-container|tertiary|gradient-primary|editorial-shadow|glass-header|btn-shimmer|ai-accent|macro-|chart-/

describe("batch primitives (F2-AC5)", () => {
  it("render no legacy class names", async () => {
    const user = userEvent.setup()
    const surplus = { ...mockSession, id: 9, surplus: 2, frozen_portions: 1 }
    render(
      <BatchHighlightProvider>
        <CookStrip
          batches={colourBatches([mockSession, surplus], mockPlan.slots)}
        />
        <BatchBadge sessionId={7} colour={1} variant="cook" portions={6} />
        <BatchBadge
          sessionId={7}
          colour={1}
          variant="leftover"
          position={2}
          meals={3}
        />
        <BatchBadge
          sessionId={7}
          colour={1}
          variant="frozen"
          position={3}
          meals={3}
        />
        <BatchBadge sessionId={7} colour={1} variant="surplus" count={2} />
        <BatchBadge sessionId={7} colour={1} variant="toFreeze" count={2} />
        <DayMeter
          day={{ ...mockPlan.days[2], complete: false, missing: ["Onion"] }}
          targets={mockPlan.targets}
        />
        <DayMeter day={mockPlan.days[0]} targets={null} />
      </BatchHighlightProvider>
    )
    // Open one popover so its portal content is checked too.
    await user.click(screen.getAllByRole("button", { name: /^Nutrition/ })[0])
    await screen.findByRole("dialog")

    const classes = [...document.body.querySelectorAll("[class]")].map(
      (el) => el.getAttribute("class") ?? ""
    )
    expect(classes.length).toBeGreaterThan(10)
    expect(classes.filter((c) => LEGACY.test(c))).toEqual([])
  })
})
