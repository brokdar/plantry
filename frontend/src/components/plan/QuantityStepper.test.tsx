import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import i18n from "@/lib/i18n"

import { QuantityStepper } from "./QuantityStepper"

beforeEach(async () => {
  await i18n.changeLanguage("en")
})

function renderStepper(value: number, onChange = vi.fn()) {
  render(
    <QuantityStepper
      label="Portions"
      value={value}
      min={0.25}
      max={10}
      step={0.25}
      onChange={onChange}
    />
  )
  return onChange
}

describe("QuantityStepper", () => {
  it("steps by a quarter and shows fractions", async () => {
    const onChange = renderStepper(1.25)
    expect(screen.getByRole("group", { name: "Portions" })).toHaveTextContent(
      "1¼"
    )
    await userEvent.click(
      screen.getByRole("button", { name: "Increase Portions" })
    )
    expect(onChange).toHaveBeenLastCalledWith(1.5)
    await userEvent.click(
      screen.getByRole("button", { name: "Decrease Portions" })
    )
    expect(onChange).toHaveBeenLastCalledWith(1)
  })

  it("stops at its bounds", () => {
    renderStepper(0.25)
    expect(
      screen.getByRole("button", { name: "Decrease Portions" })
    ).toBeDisabled()
  })

  it("steps an off-grid value to the next grid point, not past it", async () => {
    const onChange = vi.fn()
    render(
      <QuantityStepper
        label="Batch"
        value={3.5}
        min={1}
        max={100}
        step={1}
        onChange={onChange}
      />
    )
    await userEvent.click(
      screen.getByRole("button", { name: "Increase Batch" })
    )
    expect(onChange).toHaveBeenLastCalledWith(4)
    await userEvent.click(
      screen.getByRole("button", { name: "Decrease Batch" })
    )
    expect(onChange).toHaveBeenLastCalledWith(3)
  })

  it("clamps and snaps to the step grid", async () => {
    const onChange = vi.fn()
    render(
      <QuantityStepper
        label="Eaters"
        value={19.7}
        min={0.5}
        max={20}
        step={0.5}
        onChange={onChange}
      />
    )
    await userEvent.click(
      screen.getByRole("button", { name: "Increase Eaters" })
    )
    expect(onChange).toHaveBeenLastCalledWith(20)
  })
})
