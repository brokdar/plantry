import { act, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it } from "vitest"

import i18n from "@/lib/i18n"
import type { PlanDay } from "@/lib/api/plan"
import { mockPlan } from "@/test/fixtures"

import { DayMeter } from "./DayMeter"

const targets = mockPlan.targets!

function day(kcal: number, patch: Partial<PlanDay> = {}): PlanDay {
  return {
    date: "2026-11-02",
    macros: { kcal, protein: 90, fat: 60, carbs: 200 },
    complete: true,
    missing: [],
    ...patch,
  }
}

afterEach(async () => {
  await act(() => i18n.changeLanguage("en"))
})

describe("DayMeter (F2-AC4)", () => {
  it.each([
    [1430, "warning"], // 65 %
    [2200, "success"], // 100 %
    [2640, "warning"], // 120 %
    [1540, "success"], // 70 %, the lower bound
    [2530, "success"], // 115 %, the upper bound
  ])("%s kcal of 2,200 → %s", (kcal, status) => {
    render(<DayMeter day={day(kcal)} targets={targets} />)
    const meter = screen.getByRole("meter")
    expect(meter).toHaveAttribute("data-status", status)
    const fill = meter.firstElementChild!
    expect(fill).toHaveClass(status === "success" ? "bg-success" : "bg-warning")
  })

  it("exposes value text and bounds", () => {
    render(<DayMeter day={day(1840)} targets={targets} />)
    const meter = screen.getByRole("meter")
    expect(meter).toHaveAttribute("aria-valuetext", "1,840 of 2,200 kcal")
    expect(meter).toHaveAttribute("aria-valuenow", "1840")
    expect(meter).toHaveAttribute("aria-valuemin", "0")
    expect(meter).toHaveAttribute("aria-valuemax", "2200")
  })

  it("reads the value in German", async () => {
    await act(() => i18n.changeLanguage("de"))
    render(<DayMeter day={day(1840)} targets={targets} />)
    expect(screen.getByRole("meter")).toHaveAttribute(
      "aria-valuetext",
      "1.840 von 2.200 kcal"
    )
  })

  it("caps the fill at the full width", () => {
    render(<DayMeter day={day(2640)} targets={targets} />)
    const fill = screen.getByRole("meter").firstElementChild as HTMLElement
    expect(fill.style.width).toBe("100%")
  })

  it("shows a neutral bar without a target", () => {
    render(<DayMeter day={day(1840)} targets={null} />)
    const meter = screen.getByRole("meter")
    expect(meter).toHaveAttribute("data-status", "neutral")
    expect(meter).toHaveAttribute("aria-valuetext", "1,840 kcal")
    expect(meter.firstElementChild).toHaveClass("bg-ink-muted")
  })

  it("marks an incomplete day with a warning dot and says so in the popover", async () => {
    const user = userEvent.setup()
    render(
      <DayMeter
        day={day(1840, { complete: false, missing: ["Kidney beans"] })}
        targets={targets}
      />
    )
    expect(screen.getByTestId("day-meter-incomplete")).toHaveClass("bg-warning")

    await user.click(
      screen.getByRole("button", { name: "Nutrition, Monday, November 2" })
    )
    const dialog = await screen.findByRole("dialog")
    expect(dialog).toHaveTextContent("Incomplete")
    expect(dialog).toHaveTextContent("No nutrition data for Kidney beans.")
  })

  it("has no warning dot on a complete day", () => {
    render(<DayMeter day={day(1840)} targets={targets} />)
    expect(screen.queryByTestId("day-meter-incomplete")).toBeNull()
  })
})
