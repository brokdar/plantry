import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import "@/lib/i18n"

import { householdCounts, householdPortions } from "./household"
import { HouseholdHelper } from "./HouseholdHelper"

describe("household portions", () => {
  it("counts adults as 1 and children as 0.5", () => {
    expect(householdPortions(2, 1)).toBe(2.5)
  })

  it("splits stored portions into adults and children", () => {
    expect(householdCounts(2.5)).toEqual({ adults: 2, children: 1 })
    expect(householdCounts(1)).toEqual({ adults: 1, children: 0 })
  })
})

describe("HouseholdHelper", () => {
  it("adds a child as half a portion", async () => {
    const onChange = vi.fn()
    render(<HouseholdHelper value={2} onChange={onChange} />)
    await userEvent.click(
      screen.getByRole("button", { name: "Increase Children" })
    )
    expect(onChange).toHaveBeenLastCalledWith(2.5)
    expect(screen.getByRole("group", { name: "Children" })).toHaveTextContent(
      "1"
    )
  })

  it("never goes to zero portions", async () => {
    const onChange = vi.fn()
    render(<HouseholdHelper value={1} onChange={onChange} />)
    expect(
      screen.getByRole("button", { name: "Decrease Adults" })
    ).toBeDisabled()
    expect(onChange).not.toHaveBeenCalled()
  })
})
