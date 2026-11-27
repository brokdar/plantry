import { render, screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import i18n from "@/lib/i18n"
import { ImportCheckSummary } from "./ImportCheckSummary"

describe("ImportCheckSummary", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("en")
  })

  it("shows the cap warning alone when the check found nothing", () => {
    render(
      <ImportCheckSummary
        check={{
          ran: true,
          changes: [],
          flags: [],
          cap_warning: true,
        }}
        undone={new Set()}
        onUndo={vi.fn()}
        touchAdded={false}
        onAddTouch={vi.fn()}
      />
    )
    expect(screen.getByTestId("import-check")).toBeVisible()
    expect(screen.getByText(/budget/i)).toBeVisible()
    expect(screen.queryByText(/^Checked:/)).toBeNull()
  })
})
