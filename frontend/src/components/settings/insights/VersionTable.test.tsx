import { render, screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"
import "@/lib/i18n"

import { mockInsights } from "@/test/fixtures"

import { VersionTable } from "./VersionTable"

describe("VersionTable", () => {
  it("shows score components and violations per prompt version", () => {
    render(
      <VersionTable
        groups={[
          {
            ...mockInsights.by_version[0],
            score_avg: {
              coverage: 1,
              preference_fit: 3.5,
              "macro_deviation.weekly_pct": 3.25,
              "variety.distinct": 6,
              waste_portions: 0,
              "cook_occasions.per_day.2026-11-01": 1,
            },
            violations: { H6: 2, H2: 1 },
          },
        ]}
      />
    )
    const [, row] = screen.getAllByRole("row")
    expect(
      within(row)
        .getAllByRole("cell")
        .map((c) => c.textContent)
    ).toEqual([
      "Generate",
      "v1",
      "heuristic",
      "2",
      "–",
      "100%",
      "3.5",
      "3.3",
      "6",
      "0",
      "–",
      "Cooking per day × 1, Confirmed plates kept × 2",
    ])
  })

  it("keeps a planning version whose runs all failed", () => {
    render(
      <VersionTable
        groups={[
          {
            kind: "generate",
            prompt_version: "p1",
            model: "fake",
            runs: 1,
            tokens: { input: 0, output: 0, cache_read: 0, cache_write: 0 },
            edits: null,
            score_avg: {},
            violations: {},
          },
        ]}
      />
    )
    const [, row] = screen.getAllByRole("row")
    expect(within(row).getAllByRole("cell")[1]).toHaveTextContent("p1")
  })

  it("names an unknown rule by its id", () => {
    render(
      <VersionTable
        groups={[{ ...mockInsights.by_version[0], violations: { H9: 1 } }]}
      />
    )
    expect(screen.getByText("H9 × 1")).toBeInTheDocument()
  })

  it("shows a chat group without a score, with its edits", () => {
    render(
      <VersionTable
        groups={[
          {
            kind: "chat",
            prompt_version: "v5",
            model: "fake",
            runs: 2,
            tokens: { input: 0, output: 0, cache_read: 0, cache_write: 0 },
            edits: 7,
            score_avg: {},
            violations: {},
          },
        ]}
      />
    )
    const [, row] = screen.getAllByRole("row")
    const cells = within(row)
      .getAllByRole("cell")
      .map((c) => c.textContent)
    expect(cells.slice(0, 5)).toEqual(["Chat", "v5", "fake", "2", "7"])
  })

  it("says so when there are no runs at all", () => {
    render(<VersionTable groups={[]} />)
    expect(screen.getByText("No assistant runs yet.")).toBeInTheDocument()
  })
})
