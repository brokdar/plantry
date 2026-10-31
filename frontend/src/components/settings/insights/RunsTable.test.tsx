import { screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { mockRunsPage } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/runs", () => ({ listRuns: vi.fn(), getRun: vi.fn() }))

import { listRuns } from "@/lib/api/runs"

import { RunsTable } from "./RunsTable"

describe("RunsTable", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(listRuns).mockImplementation(async ({ cursor } = {}) =>
      cursor
        ? { items: [{ ...mockRunsPage.items[0], id: 3 }], next_cursor: null }
        : mockRunsPage
    )
  })

  it("renders every column of a run", async () => {
    renderWithRouter(<RunsTable onOpen={vi.fn()} />)

    const row = await screen.findByTestId(`run-row-${mockRunsPage.items[0].id}`)
    const cells = within(row)
      .getAllByRole("cell")
      .map((c) => c.textContent)
    expect(cells).toEqual([
      expect.stringMatching(/Oct 30, 2026/),
      "Generate",
      "Without the assistant",
      "v1",
      "0",
      "$0.00",
      "Done",
      "100% covered · 0 violations",
      "Accepted",
    ])
  })

  it('shows "cost unknown" for an unpriced run, tokens still visible (F7-AC2)', async () => {
    renderWithRouter(<RunsTable onOpen={vi.fn()} />)

    const row = await screen.findByTestId("run-row-11")
    expect(within(row).getByText("cost unknown")).toBeInTheDocument()
    expect(within(row).getByText("1,500")).toBeInTheDocument()
    expect(within(row).getByText("fake")).toBeInTheDocument()
  })

  it("pages by cursor", async () => {
    renderWithRouter(<RunsTable onOpen={vi.fn()} />)

    await userEvent.click(await screen.findByRole("button", { name: "Older" }))
    expect(await screen.findByTestId("run-row-3")).toBeInTheDocument()
    expect(listRuns).toHaveBeenLastCalledWith({ cursor: 11 })
    expect(screen.getByRole("button", { name: "Older" })).toBeDisabled()

    await userEvent.click(screen.getByRole("button", { name: "Newer" }))
    expect(await screen.findByTestId("run-row-11")).toBeInTheDocument()
  })

  it("opens a run by its date button", async () => {
    const onOpen = vi.fn()
    renderWithRouter(<RunsTable onOpen={onOpen} />)

    const row = await screen.findByTestId("run-row-11")
    await userEvent.click(within(row).getByRole("button"))
    expect(onOpen).toHaveBeenCalledTimes(1)
    expect(onOpen).toHaveBeenCalledWith(11)
  })
})
