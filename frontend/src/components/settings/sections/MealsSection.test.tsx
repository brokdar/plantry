import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import {
  mockAvailabilityDefaults,
  mockRoutines,
  mockTimeSlots,
} from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/availability", () => ({
  getAvailabilityDefaults: vi.fn(),
  setAvailabilityDefaults: vi.fn(),
  getAvailabilityRange: vi.fn(),
  setAvailabilityOverride: vi.fn(),
  clearAvailabilityOverride: vi.fn(),
}))
vi.mock("@/lib/api/slots", () => ({
  listTimeSlots: vi.fn(),
  createTimeSlot: vi.fn(),
  updateTimeSlot: vi.fn(),
  deleteTimeSlot: vi.fn(),
}))
vi.mock("@/lib/api/presets", () => ({
  getPresets: vi.fn(),
  getKnownTags: vi.fn().mockResolvedValue({ items: [] }),
}))
vi.mock("@/lib/api/foods", () => ({
  listFoods: vi.fn().mockResolvedValue({ items: [], total: 0 }),
}))

import {
  getAvailabilityDefaults,
  setAvailabilityDefaults,
} from "@/lib/api/availability"
import { getPresets } from "@/lib/api/presets"
import { listTimeSlots } from "@/lib/api/slots"

import { MealsSection } from "./MealsSection"

describe("MealsSection", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(listTimeSlots).mockResolvedValue({ items: mockTimeSlots })
    vi.mocked(getAvailabilityDefaults).mockResolvedValue({
      items: mockAvailabilityDefaults,
    })
    vi.mocked(setAvailabilityDefaults).mockImplementation(async (items) => ({
      items,
    }))
    vi.mocked(getPresets).mockResolvedValue({
      items: mockRoutines,
      total: mockRoutines.length,
      known_tags: [],
    })
  })

  it("renders the slot editor, the usual week from the defaults and the routines editor", async () => {
    renderWithRouter(<MealsSection />)
    expect(
      await screen.findByRole("grid", { name: "Your usual week" })
    ).toBeInTheDocument()
    expect(
      await screen.findByRole("button", { name: "Monday lunch, No plate" })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Saturday lunch, Plan" })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("button", {
        name: "Tuesday breakfast, Routine · Overnight oats",
      })
    ).toBeInTheDocument()
    expect(screen.getByLabelText(/translation key/i)).toBeInTheDocument()
    expect(screen.getByTestId("routines-editor")).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "New routine" })
    ).toBeInTheDocument()
  })

  it("saves the whole grid through PUT /defaults when a cell changes", async () => {
    const user = userEvent.setup()
    renderWithRouter(<MealsSection />)
    await user.click(
      await screen.findByRole("button", { name: "Monday dinner, Plan" })
    )
    await waitFor(() => expect(setAvailabilityDefaults).toHaveBeenCalled())
    const items = vi.mocked(setAvailabilityDefaults).mock.calls[0][0]
    expect(items).toHaveLength(21)
    expect(items).toContainEqual({
      weekday: 0,
      slot_id: 3,
      mode: "none",
      preset_id: null,
    })
    expect(items).toContainEqual({
      weekday: 1,
      slot_id: 1,
      mode: "routine",
      preset_id: 7,
    })
    expect(await screen.findByText("Saved")).toBeInTheDocument()
  })
})
