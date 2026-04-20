import { describe, expect, test } from "vitest"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import "@/lib/i18n"

import { LookupDebugPanel } from "./LookupDebugPanel"
import type { TraceEntry } from "@/lib/api/lookup"

describe("LookupDebugPanel", () => {
  test("renders nothing for an empty trace", () => {
    const { container } = render(<LookupDebugPanel trace={[]} />)
    expect(container.firstChild).toBeNull()
  })

  test("shows one row per entry with step label + duration pill", () => {
    const trace: TraceEntry[] = [
      {
        step: "ai.translate",
        level: "success",
        summary: "Hähnchenbrust → chicken breast raw",
        duration_ms: 240,
        detail: { translated: "chicken breast raw" },
      },
      {
        step: "fdc.search",
        level: "success",
        summary: "FDC returned 3 results",
        duration_ms: 820,
      },
    ]
    render(<LookupDebugPanel trace={trace} />)

    expect(screen.getByText("AI Translate")).toBeInTheDocument()
    expect(screen.getByText("FDC Search")).toBeInTheDocument()
    expect(screen.getByText("240 ms")).toBeInTheDocument()
    expect(screen.getByText("820 ms")).toBeInTheDocument()
  })

  test("expands detail payload on click when present", async () => {
    const user = userEvent.setup()
    const trace: TraceEntry[] = [
      {
        step: "ai.pick_best",
        level: "success",
        summary: "AI picked index 1",
        detail: { picked_index: 1, raw_response: "[1]" },
      },
    ]
    render(<LookupDebugPanel trace={trace} />)

    // Row is a button; clicking expands the JSON detail block.
    const row = screen.getByRole("button", {
      name: /ai pick best/i,
    })
    await user.click(row)

    expect(screen.getByText(/picked_index/)).toBeInTheDocument()
    expect(screen.getByText(/"raw_response"/)).toBeInTheDocument()
  })
})
