import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { Sparkline, type SparkPoint } from "./Sparkline"

const points: SparkPoint[] = [
  { key: "a", label: "Week 1", value: 0.5, text: "50%" },
  { key: "b", label: "Week 2", value: null, text: "" },
  { key: "c", label: "Week 3", value: 0.25, text: "25%" },
]

describe("Sparkline", () => {
  it("names every point in text, including weeks without data", () => {
    render(
      <Sparkline
        title="Correction rate"
        points={points}
        latest="25%"
        emptyText="No data"
      />
    )
    expect(
      screen.getByRole("img", {
        name: "Correction rate. Week 1: 50%; Week 2: No data; Week 3: 25%",
      })
    ).toBeInTheDocument()
    expect(screen.getByText("25%")).toHaveClass("tabular-nums")
  })

  it("breaks the line at a week without data", () => {
    const { container } = render(
      <Sparkline title="t" points={points} latest="" emptyText="–" />
    )
    // Two one-point segments, one marker each with a hover title.
    expect(container.querySelectorAll("polyline")).toHaveLength(2)
    expect(container.querySelectorAll("circle")).toHaveLength(2)
    expect(container.querySelector("circle title")?.textContent).toBe(
      "Week 1: 50%"
    )
  })

  it("shows its note", () => {
    render(
      <Sparkline
        title="t"
        points={points}
        latest=""
        emptyText="–"
        note="Excludes 2 runs without a price"
      />
    )
    expect(
      screen.getByText("Excludes 2 runs without a price")
    ).toBeInTheDocument()
  })
})
