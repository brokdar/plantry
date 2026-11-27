import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/taste", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/taste")>()),
  proposeStatements: vi.fn(),
  confirmStatement: vi.fn(),
}))
import {
  confirmStatement,
  proposeStatements,
  type Statement,
} from "@/lib/api/taste"

import { ColdStart } from "./ColdStart"

const thai: Statement = {
  kind: "taste",
  level: "cuisine",
  key: "thai",
  polarity: "love",
  window_start: "",
}

describe("ColdStart", () => {
  beforeEach(() => {
    vi.mocked(proposeStatements).mockReset()
    vi.mocked(confirmStatement).mockReset().mockResolvedValue({})
  })

  // COLD-1: the answer proposes chips; a tap stores, never the answer itself.
  it("proposes chips for the answer and stores only on a tap", async () => {
    vi.mocked(proposeStatements).mockResolvedValue({ statements: [thai] })
    renderWithRouter(<ColdStart />)
    await userEvent.type(
      await screen.findByTestId("cold-start-text"),
      "we love thai"
    )
    await userEvent.click(screen.getByTestId("cold-start-submit"))
    await waitFor(() =>
      expect(proposeStatements).toHaveBeenCalledWith("we love thai")
    )
    expect(confirmStatement).not.toHaveBeenCalled()
    expect(screen.queryByTestId("assistant-statement-week")).toBeNull()
    await userEvent.click(
      await screen.findByTestId("assistant-statement-always")
    )
    await waitFor(() =>
      expect(confirmStatement).toHaveBeenCalledWith(thai, "always")
    )
  })

  it("says so when nothing could be picked out", async () => {
    vi.mocked(proposeStatements).mockResolvedValue({ statements: [] })
    renderWithRouter(<ColdStart />)
    await userEvent.type(await screen.findByTestId("cold-start-text"), "hm")
    await userEvent.click(screen.getByTestId("cold-start-submit"))
    expect(await screen.findByText(/Nothing I could pick out/)).toBeVisible()
  })
})
