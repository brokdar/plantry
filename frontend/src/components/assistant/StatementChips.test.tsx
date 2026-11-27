import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/taste", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/taste")>()),
  confirmStatement: vi.fn(),
}))
import { confirmStatement, type Statement } from "@/lib/api/taste"

import { StatementChips } from "./StatementChips"

const taste: Statement = {
  kind: "taste",
  level: "hero",
  key: "coriander",
  polarity: "rather_not",
  window_start: "2026-03-02",
}

describe("StatementChips", () => {
  beforeEach(() => {
    vi.mocked(confirmStatement).mockReset().mockResolvedValue({})
  })

  it("[Always] confirms the statement for good", async () => {
    renderWithRouter(<StatementChips statement={taste} />)
    await userEvent.click(
      await screen.findByTestId("assistant-statement-always")
    )
    await waitFor(() =>
      expect(confirmStatement).toHaveBeenCalledWith(taste, "always")
    )
    expect(await screen.findByText("Kept for good.")).toBeVisible()
  })

  it("[Just this week] keeps it for the window", async () => {
    renderWithRouter(<StatementChips statement={taste} />)
    await userEvent.click(await screen.findByTestId("assistant-statement-week"))
    await waitFor(() =>
      expect(confirmStatement).toHaveBeenCalledWith(taste, "window")
    )
  })

  it("a health statement offers only [Always]", async () => {
    renderWithRouter(
      <StatementChips statement={{ kind: "health", word: "peanuts" }} />
    )
    expect(await screen.findByText(/peanuts/)).toBeVisible()
    expect(screen.queryByTestId("assistant-statement-week")).toBeNull()
    await userEvent.click(
      await screen.findByTestId("assistant-statement-always")
    )
    await waitFor(() =>
      expect(confirmStatement).toHaveBeenCalledWith(
        { kind: "health", word: "peanuts" },
        "always"
      )
    )
  })
})
