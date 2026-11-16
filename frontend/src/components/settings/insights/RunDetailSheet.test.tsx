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

  it("shows duration, cache and reasoning totals and per-call metrics", async () => {
    const [llmStep, ...rest] = mockRunDetail.steps
    vi.mocked(getRun).mockResolvedValue({
      ...mockRunDetail,
      duration_ms: 14230,
      tokens: {
        ...mockRunDetail.tokens,
        cache_read: 900,
        cache_write: 30,
        reasoning: 20,
      },
      steps: [
        {
          ...llmStep,
          ttft_ms: 420,
          served_model: "claude-served-1",
          thinking_chars: 144,
          tokens: {
            ...llmStep.tokens,
            cache_read: 900,
            cache_write: 30,
            reasoning: 20,
          },
        },
        ...rest,
      ],
    })
    renderWithRouter(<RunDetailSheet runId={11} onClose={vi.fn()} />)

    const metrics = await screen.findByTestId("step-metrics")
    expect(metrics).toHaveTextContent("first token after 420 ms")
    expect(metrics).toHaveTextContent("cache 900 read / 30 written")
    expect(metrics).toHaveTextContent("20 reasoning tokens")
    expect(metrics).toHaveTextContent("144 characters of thinking")
    expect(metrics).toHaveTextContent("served by claude-served-1")
    expect(screen.getByText("14,230 ms")).toBeInTheDocument()
    expect(
      screen.getByText(/900 read from cache, 30 written to cache/)
    ).toBeInTheDocument()
  })

  it("shows the reasoning summary of a call, also from a cut-off response", async () => {
    const [llmStep, ...rest] = mockRunDetail.steps
    const cut =
      '{"content":[{"type":"thinking","thinking":"Weighed the options.","signature":"S"},{"type":"text","text":"Hel'
    vi.mocked(getRun).mockResolvedValue({
      ...mockRunDetail,
      steps: [{ ...llmStep, thinking_chars: 19, result: cut }, ...rest],
    })
    renderWithRouter(<RunDetailSheet runId={11} onClose={vi.fn()} />)
    expect(await screen.findByTestId("step-thinking")).toHaveTextContent(
      "Weighed the options."
    )
  })

  it("Z1: shows the run's edits, a dash when null", async () => {
    vi.mocked(getRun).mockResolvedValue({ ...mockRunDetail, edits: 3 })
    const { unmount } = renderWithRouter(
      <RunDetailSheet runId={11} onClose={vi.fn()} />
    )
    expect(await screen.findByTestId("run-detail-edits")).toHaveTextContent(
      "Edits3"
    )
    unmount()

    vi.mocked(getRun).mockResolvedValue({ ...mockRunDetail, edits: null })
    renderWithRouter(<RunDetailSheet runId={11} onClose={vi.fn()} />)
    expect(await screen.findByTestId("run-detail-edits")).toHaveTextContent(
      "Edits–"
    )
  })

  it("F4-AC5: shows a run's web searches only when it made some", async () => {
    vi.mocked(getRun).mockResolvedValue({
      ...mockRunDetail,
      web_search_requests: 2,
    })
    const { unmount } = renderWithRouter(
      <RunDetailSheet runId={11} onClose={vi.fn()} />
    )
    expect(await screen.findByTestId("run-detail-searches")).toHaveTextContent(
      "Web searches2"
    )
    unmount()

    vi.mocked(getRun).mockResolvedValue(mockRunDetail)
    renderWithRouter(<RunDetailSheet runId={11} onClose={vi.fn()} />)
    await screen.findByTestId("run-detail-edits")
    expect(screen.queryByTestId("run-detail-searches")).toBeNull()
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

  it("links a regenerate step to its run", async () => {
    vi.mocked(getRun).mockResolvedValue({
      ...mockRunDetail,
      steps: [
        {
          ...mockRunDetail.steps[1],
          name: "regenerate",
          result: '{"run_id":42,"status":"ok"}',
          result_truncated: false,
        },
      ],
    })
    renderWithRouter(<RunDetailSheet runId={11} onClose={vi.fn()} />)

    const link = await screen.findByTestId("step-run-link")
    expect(link).toHaveAttribute("href", "/settings/assistant/insights?run=42")
  })

  it("has no run link on other steps", async () => {
    renderWithRouter(<RunDetailSheet runId={11} onClose={vi.fn()} />)
    await screen.findAllByTestId("run-step")
    expect(screen.queryByTestId("step-run-link")).toBeNull()
  })

  it("stays closed without a run", () => {
    renderWithRouter(<RunDetailSheet runId={undefined} onClose={vi.fn()} />)
    expect(screen.queryByTestId("run-detail-sheet")).toBeNull()
    expect(getRun).not.toHaveBeenCalled()
  })
})
