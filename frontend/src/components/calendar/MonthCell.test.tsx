import { render, screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import "@/lib/i18n"
import type { Plate } from "@/lib/api/plates"

import { MonthCell } from "./MonthCell"

vi.mock("@/lib/api/plates")

function makePlate(id: number, note: string | null = null): Plate {
  return {
    id,
    slot_id: 1,
    date: "2026-04-26",
    note,
    skipped: false,
    components: [],
    created_at: "2026-04-26T00:00:00Z",
  }
}

const noop = vi.fn()

describe("MonthCell", () => {
  it("shows up to 3 plate previews", () => {
    const plates = [
      makePlate(1, "Pasta"),
      makePlate(2, "Salad"),
      makePlate(3, "Soup"),
    ]
    render(
      <MonthCell
        date="2026-04-26"
        plates={plates}
        isCurrentMonth={true}
        isToday={false}
        matchesSearch={null}
        onClick={noop}
      />
    )
    expect(screen.getByText("Pasta")).toBeInTheDocument()
    expect(screen.getByText("Salad")).toBeInTheDocument()
    expect(screen.getByText("Soup")).toBeInTheDocument()
    expect(screen.queryByText(/more/)).not.toBeInTheDocument()
  })

  it("shows +N more overflow when 4+ plates provided", () => {
    const plates = [
      makePlate(1, "Pasta"),
      makePlate(2, "Salad"),
      makePlate(3, "Soup"),
      makePlate(4, "Toast"),
      makePlate(5, "Oats"),
    ]
    render(
      <MonthCell
        date="2026-04-26"
        plates={plates}
        isCurrentMonth={true}
        isToday={false}
        matchesSearch={null}
        onClick={noop}
      />
    )
    expect(screen.getByText("Pasta")).toBeInTheDocument()
    expect(screen.getByText("Salad")).toBeInTheDocument()
    expect(screen.getByText("Soup")).toBeInTheDocument()
    expect(screen.queryByText("Toast")).not.toBeInTheDocument()
    expect(screen.getByText("+2 more")).toBeInTheDocument()
  })

  it("highlights with border-primary when isToday=true", () => {
    const { container } = render(
      <MonthCell
        date="2026-04-26"
        plates={[]}
        isCurrentMonth={true}
        isToday={true}
        matchesSearch={null}
        onClick={noop}
      />
    )
    const button = container.querySelector("button")!
    expect(button.className).toMatch(/border-2/)
    expect(button.className).toMatch(/border-primary/)
  })

  it("does not apply ring when isToday=false", () => {
    const { container } = render(
      <MonthCell
        date="2026-04-26"
        plates={[]}
        isCurrentMonth={true}
        isToday={false}
        matchesSearch={null}
        onClick={noop}
      />
    )
    const button = container.querySelector("button")!
    expect(button.className).not.toMatch(/ring-2/)
  })

  it("mutes date number when isCurrentMonth=false", () => {
    const { container } = render(
      <MonthCell
        date="2026-03-31"
        plates={[]}
        isCurrentMonth={false}
        isToday={false}
        matchesSearch={null}
        onClick={noop}
      />
    )
    const span = container.querySelector("button > span")!
    expect(span.className).toMatch(/text-on-surface-variant/)
  })

  it("dims (opacity-40) when matchesSearch=false", () => {
    const { container } = render(
      <MonthCell
        date="2026-04-26"
        plates={[makePlate(1, "Pasta")]}
        isCurrentMonth={true}
        isToday={false}
        matchesSearch={false}
        onClick={noop}
      />
    )
    const button = container.querySelector("button")!
    expect(button.className).toMatch(/opacity-40/)
  })

  it("full opacity when matchesSearch=true", () => {
    const { container } = render(
      <MonthCell
        date="2026-04-26"
        plates={[makePlate(1, "Pasta")]}
        isCurrentMonth={true}
        isToday={false}
        matchesSearch={true}
        onClick={noop}
      />
    )
    const button = container.querySelector("button")!
    expect(button.className).not.toMatch(/opacity-40/)
  })

  it("full opacity when matchesSearch=null", () => {
    const { container } = render(
      <MonthCell
        date="2026-04-26"
        plates={[makePlate(1, "Pasta")]}
        isCurrentMonth={true}
        isToday={false}
        matchesSearch={null}
        onClick={noop}
      />
    )
    const button = container.querySelector("button")!
    expect(button.className).not.toMatch(/opacity-40/)
  })
})
