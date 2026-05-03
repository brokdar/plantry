import { describe, expect, test, vi } from "vitest"
import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithRouter } from "@/test/render"
import { DateRangeNavigator } from "./DateRangeNavigator"

const defaultProps = {
  from: "2026-04-26",
  to: "2026-05-02",
  onPrev: vi.fn(),
  onNext: vi.fn(),
  onToday: vi.fn(),
}

describe("DateRangeNavigator", () => {
  test("renders the range label", async () => {
    renderWithRouter(<DateRangeNavigator {...defaultProps} />)
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
    await user.click(await screen.findByRole("button", { name: "Today" }))
    expect(onToday).toHaveBeenCalledTimes(1)
  })
})
