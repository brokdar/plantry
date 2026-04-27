import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { renderWithRouter } from "@/test/render"

import { CalendarHeader } from "./CalendarHeader"

vi.mock("@/lib/api/plates")

function defaultProps() {
  return {
    mode: "month" as const,
    label: "April 2026",
    onPrev: vi.fn(),
    onNext: vi.fn(),
    onToday: vi.fn(),
    onModeChange: vi.fn(),
    search: "",
    onSearchChange: vi.fn(),
    onJumpToDate: vi.fn(),
    jumpValue: "",
  }
}

describe("CalendarHeader", () => {
  it("mode toggle buttons call onModeChange with month, week, agenda", async () => {
    const user = userEvent.setup()
    const props = defaultProps()
    renderWithRouter(<CalendarHeader {...props} />)

    await user.click(await screen.findByRole("button", { name: "Month" }))
    expect(props.onModeChange).toHaveBeenCalledWith("month")

    await user.click(screen.getByRole("button", { name: "Week" }))
    expect(props.onModeChange).toHaveBeenCalledWith("week")

    await user.click(screen.getByRole("button", { name: "Agenda" }))
    expect(props.onModeChange).toHaveBeenCalledWith("agenda")
  })

  it("prev button calls onPrev", async () => {
    const user = userEvent.setup()
    const props = defaultProps()
    renderWithRouter(<CalendarHeader {...props} />)

    await user.click(await screen.findByRole("button", { name: "Previous" }))
    expect(props.onPrev).toHaveBeenCalledOnce()
  })

  it("next button calls onNext", async () => {
    const user = userEvent.setup()
    const props = defaultProps()
    renderWithRouter(<CalendarHeader {...props} />)

    await user.click(await screen.findByRole("button", { name: "Next" }))
    expect(props.onNext).toHaveBeenCalledOnce()
  })

  it("today button calls onToday", async () => {
    const user = userEvent.setup()
    const props = defaultProps()
    renderWithRouter(<CalendarHeader {...props} />)

    await user.click(await screen.findByRole("button", { name: "Today" }))
    expect(props.onToday).toHaveBeenCalledOnce()
  })

  it("search input change propagates to onSearchChange", async () => {
    const user = userEvent.setup()
    const props = defaultProps()
    renderWithRouter(<CalendarHeader {...props} />)

    const input = await screen.findByPlaceholderText("Search dishes…")
    await user.type(input, "p")
    // The component is controlled — each keystroke fires onSearchChange with the character
    expect(props.onSearchChange).toHaveBeenCalledWith("p")
  })
})
