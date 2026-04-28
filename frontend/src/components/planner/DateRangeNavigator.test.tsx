import { describe, expect, test, vi } from "vitest"
import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithRouter } from "@/test/render"
import { DateRangeNavigator } from "./DateRangeNavigator"

const defaultProps = {
  from: "2026-04-26",
  to: "2026-05-02",
  days: 7,
  planAnchor: "today" as const,
  shoppingDay: 5, // 0=Mon…6=Sun — 5 = Saturday
  onPrev: vi.fn(),
  onNext: vi.fn(),
  onToday: vi.fn(),
  onJumpToToday: vi.fn(),
}

describe("DateRangeNavigator", () => {
  test("renders the range label", async () => {
    renderWithRouter(<DateRangeNavigator {...defaultProps} />)
    // Matches "Apr 26 – May 2" (locale-formatted month+day)
    const label = await screen.findByText(/Apr 26/)
    expect(label).toBeInTheDocument()
    expect(label.textContent).toMatch(/May 2/)
  })

  test("calls onPrev when Previous button is clicked", async () => {
    const user = userEvent.setup()
    const onPrev = vi.fn()
    renderWithRouter(<DateRangeNavigator {...defaultProps} onPrev={onPrev} />)
    await user.click(await screen.findByRole("button", { name: /Previous 7/i }))
    expect(onPrev).toHaveBeenCalledTimes(1)
  })

  test("calls onNext when Next button is clicked", async () => {
    const user = userEvent.setup()
    const onNext = vi.fn()
    renderWithRouter(<DateRangeNavigator {...defaultProps} onNext={onNext} />)
    await user.click(await screen.findByRole("button", { name: /Next 7/i }))
    expect(onNext).toHaveBeenCalledTimes(1)
  })

  test("calls onToday when Today button is clicked", async () => {
    const user = userEvent.setup()
    const onToday = vi.fn()
    renderWithRouter(<DateRangeNavigator {...defaultProps} onToday={onToday} />)
    // Use exact name "Today" to avoid matching the "From today" chip
    await user.click(await screen.findByRole("button", { name: "Today" }))
    expect(onToday).toHaveBeenCalledTimes(1)
  })

  test("calls onJumpToToday when 'From today' chip is clicked", async () => {
    const user = userEvent.setup()
    const onJumpToToday = vi.fn()
    renderWithRouter(
      <DateRangeNavigator {...defaultProps} onJumpToToday={onJumpToToday} />
    )
    await user.click(await screen.findByText("From today"))
    expect(onJumpToToday).toHaveBeenCalledTimes(1)
  })

  test("shows 'From next Saturday' chip only when planAnchor is next_shopping_day", async () => {
    const { rerender } = renderWithRouter(
      <DateRangeNavigator {...defaultProps} planAnchor="today" />
    )
    // The chip must NOT appear when planAnchor is "today"
    expect(screen.queryByText(/From next Saturday/i)).not.toBeInTheDocument()

    rerender(
      <DateRangeNavigator {...defaultProps} planAnchor="next_shopping_day" />
    )
    expect(await screen.findByText(/From next Saturday/i)).toBeInTheDocument()
  })
})
