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
              preference_fit: 0.8,
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
      "100%",
      "80%",
      "3.3",
      "6",
      "0",
      "–",
      "H2 × 1, H6 × 2",
    ])
  })

  it("leaves out groups without scores, such as chat", () => {
    render(
      <VersionTable
        groups={[
          {
            kind: "chat",
            prompt_version: "v4",
            model: "fake",
            runs: 2,
            score_avg: {},
            violations: {},
          },
        ]}
      />
    )
    expect(screen.getByText("No scored plans yet.")).toBeInTheDocument()
  })
})
