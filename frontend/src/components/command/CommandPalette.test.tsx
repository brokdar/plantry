import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, test, vi } from "vitest"

import { renderWithRouter } from "@/test/render"
import { CommandPalette } from "./CommandPalette"
import { GO_TO_ENTRIES } from "./palette-entries"

vi.mock("@/lib/api/presets", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/presets")>()),
  getPresets: vi.fn(),
  applyPreset: vi.fn(),
}))
vi.mock("@/lib/api/slots", () => ({ listTimeSlots: vi.fn() }))

import { applyPreset, getPresets } from "@/lib/api/presets"
import { listTimeSlots } from "@/lib/api/slots"

const routine = {
  id: 3,
  name: "Weekday oats",
  tags: [],
  plates: [{ id: 1, slot_id: 1, sort_order: 0, components: [] }],
  created_at: "",
  updated_at: "",
}

function renderPalette() {
  renderWithRouter(
    <CommandPalette
      open
      onOpenChange={vi.fn()}
      onOpenCopyWeek={vi.fn()}
      defaultTargetDate="2026-10-26"
    />
  )
}

describe("CommandPalette", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getPresets).mockResolvedValue({
      items: [routine],
      total: 1,
      known_tags: [],
    })
    vi.mocked(listTimeSlots).mockResolvedValue({ items: [] })
  })

  // F10-AC3
  test("goes to Today, Plan, Recipes, Ingredients, Shop and Settings only", () => {
    const targets = GO_TO_ENTRIES.map((e) => e.to)
    expect(targets).toEqual([
      "/today",
      "/plan",
      "/recipes",
      "/recipes/ingredients",
      "/shop",
      "/settings",
    ])
    for (const gone of ["/calendar", "/archive", "/import", "/presets"]) {
      expect(targets.some((to) => to.startsWith(gone))).toBe(false)
    }
  })

  test("lists routines, actions and the go-to entries", async () => {
    renderPalette()
    expect(
      await screen.findByRole("option", { name: /Weekday oats/ })
    ).toBeInTheDocument()
    for (const name of [
      "Copy a previous week…",
      "Manage routines",
      "Today",
      "Plan",
      "Recipes",
      "Ingredients",
      "Shop",
      "Settings",
    ]) {
      expect(
        screen.getByRole("option", { name: new RegExp(`^${name}$`) })
      ).toBeInTheDocument()
    }
    expect(screen.queryByRole("option", { name: /^Import$/ })).toBeNull()
  })

  // F10-AC2 (unit part; e2e covers undo against the backend)
  test("applies a routine to the picked day; Shift+Enter replaces", async () => {
    vi.mocked(applyPreset).mockResolvedValue({
      created_plate_ids: [7],
      skipped_occupied: [],
      skipped_unavailable: [],
      skipped_no_slot: [],
      removed: { plates: [], sessions: [] },
      created_overrides: [],
      skipped_overrides: [],
    })
    const user = userEvent.setup()
    renderPalette()
    await user.click(
      await screen.findByRole("option", { name: /Weekday oats/ })
    )
    const day = await screen.findByLabelText("Day")
    expect(day).toHaveValue("2026-10-26")
    await user.keyboard("{Shift>}{Enter}{/Shift}")
    await waitFor(() =>
      expect(applyPreset).toHaveBeenCalledWith(3, {
        target_date: "2026-10-26",
        on_conflict: "overwrite",
        slot_ids_filter: undefined,
      })
    )
  })
})
