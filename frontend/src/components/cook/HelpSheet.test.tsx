import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ApiError } from "@/lib/api/client"
import i18n from "@/lib/i18n"
import { mockScaled } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/help", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/help")>()),
  askHelp: vi.fn(),
}))
vi.mock("@/lib/api/substitutions", () => ({
  getSubstitutes: vi.fn().mockResolvedValue({ options: [], session_ids: [7] }),
  applySubstitution: vi.fn(),
  keepSubstitution: vi.fn(),
}))
vi.mock("@/lib/api/foods", () => ({ revertFoodChange: vi.fn() }))
vi.mock("@/lib/toast", () => ({ toast: vi.fn(), toastError: vi.fn() }))

import { askHelp } from "@/lib/api/help"

import { HelpButton } from "./HelpSheet"

function renderHelp() {
  return renderWithRouter(
    <HelpButton sessionId={7} step={2} ingredients={mockScaled.ingredients} />
  )
}

describe("HelpButton", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("en")
  })

  it("is a 56 px button that opens three tiles", async () => {
    const user = userEvent.setup()
    renderHelp()
    const open = await screen.findByTestId("help-open")
    expect(open).toHaveClass("h-14")
    await user.click(open)
    expect(screen.getByTestId("help-tile-substitute")).toBeInTheDocument()
    expect(screen.getByTestId("help-tile-explain")).toBeInTheDocument()
    expect(screen.getByTestId("help-tile-wrong")).toBeInTheDocument()
  })

  it("Explain this step asks about the step on screen and shows the lines", async () => {
    vi.mocked(askHelp).mockResolvedValue({
      answer: ["Chop finely.", "Keep the pieces even."],
      run_id: 1,
    })
    const user = userEvent.setup()
    renderHelp()
    await user.click(await screen.findByTestId("help-open"))
    await user.click(screen.getByTestId("help-tile-explain"))
    expect(askHelp).toHaveBeenCalledWith({
      session_id: 7,
      step_index: 2,
      kind: "explain_step",
    })
    const answer = await screen.findByTestId("help-answer")
    expect(answer.querySelectorAll("li")).toHaveLength(2)
    expect(answer).toHaveTextContent("Chop finely.")
  })

  it("Something went wrong: a chip plus free text goes in one request", async () => {
    vi.mocked(askHelp).mockResolvedValue({ answer: ["+1 potato."], run_id: 2 })
    const user = userEvent.setup()
    renderHelp()
    await user.click(await screen.findByTestId("help-open"))
    await user.click(screen.getByTestId("help-tile-wrong"))
    expect(screen.getByTestId("help-wrong-send")).toBeDisabled()
    await user.click(screen.getByTestId("help-problem-too_salty"))
    await user.type(screen.getByTestId("help-wrong-text"), "a lot")
    await user.click(screen.getByTestId("help-wrong-send"))
    expect(askHelp).toHaveBeenCalledWith({
      session_id: 7,
      step_index: 2,
      kind: "went_wrong",
      problem: "too_salty",
      text: "a lot",
    })
    expect(await screen.findByTestId("help-answer")).toHaveTextContent(
      "+1 potato."
    )
  })

  it("shows an error inline with a retry, never a dialog", async () => {
    vi.mocked(askHelp)
      .mockRejectedValueOnce(new ApiError(502, "error.ai.provider_error"))
      .mockResolvedValueOnce({ answer: ["Stir."], run_id: 3 })
    const user = userEvent.setup()
    renderHelp()
    await user.click(await screen.findByTestId("help-open"))
    await user.click(screen.getByTestId("help-tile-explain"))
    expect(await screen.findByTestId("help-error")).toHaveTextContent(
      "couldn't get an answer"
    )
    await user.click(screen.getByTestId("help-retry"))
    await waitFor(() => expect(askHelp).toHaveBeenCalledTimes(2))
    expect(await screen.findByTestId("help-answer")).toHaveTextContent("Stir.")
  })

  it("says so when no provider is set up", async () => {
    vi.mocked(askHelp).mockRejectedValue(
      new ApiError(503, "error.ai.provider_missing")
    )
    const user = userEvent.setup()
    renderHelp()
    await user.click(await screen.findByTestId("help-open"))
    await user.click(screen.getByTestId("help-tile-explain"))
    expect(await screen.findByTestId("help-error")).toHaveTextContent(
      "isn't set up"
    )
  })

  it("I don't have an ingredient opens the existing substitute flow", async () => {
    const user = userEvent.setup()
    renderHelp()
    await user.click(await screen.findByTestId("help-open"))
    await user.click(screen.getByTestId("help-tile-substitute"))
    expect(askHelp).not.toHaveBeenCalled()
    expect(
      await screen.findByText(i18n.t("substitute.pick_title"))
    ).toBeInTheDocument()
  })
})
