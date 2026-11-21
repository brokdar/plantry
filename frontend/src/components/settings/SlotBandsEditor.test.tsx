import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { mockTimeSlots } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/slots", () => ({
  listTimeSlots: vi.fn(),
  getSlotBands: vi.fn(),
  setSlotBands: vi.fn(),
  applySlotBandPreset: vi.fn(),
}))

import {
  applySlotBandPreset,
  getSlotBands,
  listTimeSlots,
  setSlotBands,
} from "@/lib/api/slots"

import { SlotBandsEditor } from "./SlotBandsEditor"

const bands = {
  items: [
    { slot_id: 1, min_pct: 20, max_pct: 30 },
    { slot_id: 2, min_pct: 30, max_pct: 40 },
    { slot_id: 3, min_pct: 25, max_pct: 40 },
  ],
  shares: { "1": 27, "2": 37.8, "3": 35.1 },
}

describe("SlotBandsEditor", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(listTimeSlots).mockResolvedValue({ items: mockTimeSlots })
    vi.mocked(getSlotBands).mockResolvedValue(bands)
  })

  it("shows the bands and the running sums, and saves a valid change", async () => {
    vi.mocked(setSlotBands).mockResolvedValue(bands)
    renderWithRouter(<SlotBandsEditor />)
    const lunchMin = await screen.findByLabelText(/lunch.*min/i)
    expect(lunchMin).toHaveValue(30)
    expect(
      screen.getByText(/add up|ergeben|Minimums add up to 75/i)
    ).toBeTruthy()

    await userEvent.clear(lunchMin)
    await userEvent.type(lunchMin, "35")
    await userEvent.click(screen.getByRole("button", { name: /save bands/i }))
    await waitFor(() =>
      expect(setSlotBands).toHaveBeenCalledWith([
        { slot_id: 1, min_pct: 20, max_pct: 30 },
        { slot_id: 2, min_pct: 35, max_pct: 40 },
        { slot_id: 3, min_pct: 25, max_pct: 40 },
      ])
    )
  })

  it("refuses to save a set whose minimums pass 100, naming the numbers", async () => {
    renderWithRouter(<SlotBandsEditor />)
    const lunchMin = await screen.findByLabelText(/lunch.*min/i)
    await userEvent.clear(screen.getByLabelText(/lunch.*max/i))
    await userEvent.type(screen.getByLabelText(/lunch.*max/i), "70")
    await userEvent.clear(lunchMin)
    await userEvent.type(lunchMin, "60")
    const alert = screen.getByRole("status")
    await waitFor(() => expect(alert.textContent).toMatch(/105 %/))
    expect(screen.getByRole("button", { name: /save bands/i })).toBeDisabled()
    expect(setSlotBands).not.toHaveBeenCalled()
  })

  it("applies a preset", async () => {
    vi.mocked(applySlotBandPreset).mockResolvedValue(bands)
    renderWithRouter(<SlotBandsEditor />)
    await userEvent.click(
      await screen.findByRole("button", { name: /dinner is the biggest meal/i })
    )
    await waitFor(() =>
      expect(applySlotBandPreset).toHaveBeenCalledWith("dinner_biggest")
    )
  })

  it("announces the sums through a live region that is always mounted, and marks the rows at fault", async () => {
    renderWithRouter(<SlotBandsEditor />)
    const lunchMin = await screen.findByLabelText(/lunch.*min/i)
    const region = screen.getByRole("status")
    expect(region.textContent).toMatch(/75/)
    expect(lunchMin).not.toHaveAttribute("aria-invalid", "true")

    await userEvent.clear(screen.getByLabelText(/lunch.*max/i))
    await userEvent.type(screen.getByLabelText(/lunch.*max/i), "70")
    await userEvent.clear(lunchMin)
    await userEvent.type(lunchMin, "60")

    // the same element, its text changed
    expect(screen.getByRole("status")).toBe(region)
    await waitFor(() => expect(region.textContent).toMatch(/105 %/))
    expect(lunchMin).toHaveAttribute("aria-invalid", "true")
    expect(region.id).not.toBe("")
    expect(lunchMin).toHaveAttribute("aria-describedby", region.id)
  })
})
