import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"
import "@/lib/i18n"

import { PortionStepper } from "./PortionStepper"

describe("PortionStepper", () => {
  it("renders the value as an integer", () => {
    render(<PortionStepper value={3} onChange={vi.fn()} />)
    expect(screen.getByTestId("portion-stepper-value").textContent).toBe("×3")
  })

  it("rounds non-integer input to an integer for display", () => {
    render(<PortionStepper value={1.7} onChange={vi.fn()} />)
    expect(screen.getByTestId("portion-stepper-value").textContent).toBe("×2")
  })

  it("+ increments by 1", async () => {
    const onChange = vi.fn()
    render(<PortionStepper value={2} onChange={onChange} />)
    const user = userEvent.setup()
    await user.click(screen.getByRole("button", { name: /\+1/ }))
    expect(onChange).toHaveBeenCalledWith(3)
  })

  it("− decrements by 1", async () => {
    const onChange = vi.fn()
    render(<PortionStepper value={3} onChange={onChange} />)
    const user = userEvent.setup()
    await user.click(screen.getByRole("button", { name: /−1/ }))
    expect(onChange).toHaveBeenCalledWith(2)
  })

  it("cannot go below 1 (minus disabled at 1)", async () => {
    const onChange = vi.fn()
    render(<PortionStepper value={1} onChange={onChange} />)
    const user = userEvent.setup()
    const minus = screen.getByRole("button", { name: /−1/ })
    expect(minus).toBeDisabled()
    await user.click(minus)
    expect(onChange).not.toHaveBeenCalled()
  })

  it("aria-valuenow reflects the current value", () => {
    render(<PortionStepper value={4} onChange={vi.fn()} />)
    const sb = screen.getByRole("spinbutton")
    expect(sb).toHaveAttribute("aria-valuenow", "4")
    expect(sb).toHaveAttribute("aria-valuemin", "1")
  })
})
