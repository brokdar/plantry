import { screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { mockRunDetail } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/runs", () => ({ listRuns: vi.fn(), getRun: vi.fn() }))

import { getRun } from "@/lib/api/runs"

import { RunDetailSheet } from "./RunDetailSheet"

describe("RunDetailSheet", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getRun).mockResolvedValue(mockRunDetail)
  })

  it("reads the steps in plain language", async () => {
    renderWithRouter(<RunDetailSheet runId={11} onClose={vi.fn()} />)

    const steps = await screen.findAllByTestId("run-step")
    expect(steps.map((s) => s.firstElementChild?.textContent)).toEqual([
      "Asked the assistant (fake, 1,500 tokens)420 ms",
      "Checked your meal slots3 ms",
    ])
    expect(screen.getByRole("heading", { name: "Chat" })).toBeInTheDocument()
    expect(screen.getByText("cost unknown")).toBeInTheDocument()
  })

  it("keeps the raw JSON behind a disclosure", async () => {
    renderWithRouter(<RunDetailSheet runId={11} onClose={vi.fn()} />)

    const [, tool] = await screen.findAllByTestId("run-step")
    const raw = within(tool).getByText("Raw JSON")
    expect(raw.closest("details")).not.toHaveAttribute("open")
    await userEvent.click(raw)
    expect(raw.closest("details")).toHaveAttribute("open")
    expect(within(tool).getByText('{"slots":[]}')).toBeVisible()
    expect(within(tool).getByText(/Cut short/)).toBeVisible()
  })

  it("names an unknown tool by its id", async () => {
    vi.mocked(getRun).mockResolvedValue({
      ...mockRunDetail,
      steps: [{ ...mockRunDetail.steps[1], name: "brand_new_tool" }],
    })
    renderWithRouter(<RunDetailSheet runId={11} onClose={vi.fn()} />)

    expect(
      await screen.findByText("Used the tool brand_new_tool")
    ).toBeInTheDocument()
  })

  // DoD nit: the planning chooser's forced tool reads in plain language.
  it("names the submit_plan step", async () => {
    vi.mocked(getRun).mockResolvedValue({
      ...mockRunDetail,
      steps: [{ ...mockRunDetail.steps[1], name: "submit_plan" }],
    })
    renderWithRouter(<RunDetailSheet runId={11} onClose={vi.fn()} />)

    expect(await screen.findByText("Handed in the plan")).toBeInTheDocument()
  })

  it("stays closed without a run", () => {
    renderWithRouter(<RunDetailSheet runId={undefined} onClose={vi.fn()} />)
    expect(screen.queryByTestId("run-detail-sheet")).toBeNull()
    expect(getRun).not.toHaveBeenCalled()
  })
})
