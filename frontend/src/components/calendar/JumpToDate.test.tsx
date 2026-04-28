import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { renderWithRouter } from "@/test/render"

import { JumpToDate } from "./JumpToDate"

vi.mock("@/lib/api/plates")

describe("JumpToDate", () => {
  it("onSelect is called with YYYY-MM-DD string when user picks a date", async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    renderWithRouter(<JumpToDate value="" onSelect={onSelect} />)

    // Open the popover by clicking the trigger button
    const trigger = await screen.findByRole("button", { name: "Jump to date" })
    await user.click(trigger)

    // The popover exposes a date input
    const input = await screen.findByDisplayValue("")
    // Simulate the user typing a date value
    await user.type(input, "2026-05-10")

    // The onChange handler fires with the input value "2026-05-10"
    expect(onSelect).toHaveBeenCalledWith("2026-05-10")
  })

  it("shows custom label from value when value is set", async () => {
    renderWithRouter(<JumpToDate value="2026-05-10" onSelect={vi.fn()} />)

    const trigger = await screen.findByRole("button", { name: "2026-05-10" })
    expect(trigger).toBeInTheDocument()
  })
})
