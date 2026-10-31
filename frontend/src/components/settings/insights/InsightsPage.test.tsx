import { screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { mockInsights, mockRunDetail, mockRunsPage } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/runs", () => ({ listRuns: vi.fn(), getRun: vi.fn() }))
vi.mock("@/lib/api/insights", () => ({ getInsights: vi.fn() }))

import { getInsights } from "@/lib/api/insights"
import { getRun, listRuns } from "@/lib/api/runs"

import { InsightsPage } from "./InsightsPage"

describe("InsightsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(listRuns).mockResolvedValue(mockRunsPage)
    vi.mocked(getInsights).mockResolvedValue(mockInsights)
    vi.mocked(getRun).mockResolvedValue(mockRunDetail)
  })

  it("shows the three trend sparklines with text equivalents", async () => {
    renderWithRouter(<InsightsPage runId={undefined} onRunChange={vi.fn()} />)

    const correction = await screen.findByTestId("spark-correction")
    // The latest week's value as text, and every week in the SVG's name.
    expect(within(correction).getByText("25%")).toBeInTheDocument()
    expect(
      within(correction).getByRole("img", {
        name: "Correction rate. Week of Oct 24: 50%; Week of Oct 31: 25%",
      })
    ).toBeInTheDocument()
    // Acceptance = accepted / (accepted + discarded): 1/2, then 1/1.
    expect(
      within(screen.getByTestId("spark-acceptance")).getByRole("img", {
        name: "Acceptance rate. Week of Oct 24: 50%; Week of Oct 31: 100%",
      })
    ).toBeInTheDocument()
  })

  it("notes the runs the cost leaves out (F7-AC2)", async () => {
    renderWithRouter(<InsightsPage runId={undefined} onRunChange={vi.fn()} />)

    const cost = await screen.findByTestId("spark-cost")
    expect(
      within(cost).getByText("Excludes 1 run without a price")
    ).toBeInTheDocument()
    // The week's row in the table says the same.
    const week = screen.getByTestId("week-row-2026-10-31")
    expect(
      within(week).getByText("Excludes 1 run without a price")
    ).toBeInTheDocument()
    expect(
      within(screen.getByTestId("week-row-2026-10-24")).queryByText(
        /without a price/
      )
    ).toBeNull()
  })

  it("lists the runs and the by-version table", async () => {
    renderWithRouter(<InsightsPage runId={undefined} onRunChange={vi.fn()} />)

    expect(await screen.findByTestId("run-row-11")).toBeInTheDocument()
    expect(
      await screen.findByRole("table", { name: "By prompt version" })
    ).toBeInTheDocument()
    expect(screen.getByText(/Prices as of/)).toHaveTextContent(
      "Prices as of Oct 1, 2026."
    )
  })

  it("opens a run's detail from the table", async () => {
    const onRunChange = vi.fn()
    renderWithRouter(
      <InsightsPage runId={undefined} onRunChange={onRunChange} />
    )

    const row = await screen.findByTestId("run-row-11")
    await userEvent.click(within(row).getByRole("button"))
    expect(onRunChange).toHaveBeenCalledWith(11)
  })

  it("shows the run named by ?run in the sheet", async () => {
    renderWithRouter(<InsightsPage runId={11} onRunChange={vi.fn()} />)

    const sheet = await screen.findByTestId("run-detail-sheet")
    expect(
      await within(sheet).findByText("Asked the assistant (fake, 1,500 tokens)")
    ).toBeInTheDocument()
    expect(getRun).toHaveBeenCalledWith(11)
  })

  it("teaches the next step when there are no runs", async () => {
    vi.mocked(listRuns).mockResolvedValue({ items: [], next_cursor: null })
    vi.mocked(getInsights).mockResolvedValue({
      ...mockInsights,
      weeks: mockInsights.weeks.map((w) => ({
        ...w,
        runs: 0,
        accepted: 0,
        discarded: 0,
      })),
      by_version: [],
    })
    renderWithRouter(<InsightsPage runId={undefined} onRunChange={vi.fn()} />)

    expect(await screen.findByText(/No assistant runs yet/)).toBeInTheDocument()
    expect(screen.getByRole("link", { name: "Go to the plan" })).toBeVisible()
    expect(screen.getByText(/No runs in the last 12 weeks/)).toBeInTheDocument()
  })
})
