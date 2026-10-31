import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { Plan, PlanCell } from "@/lib/api/plan"
import type { ApplyResult, Preset } from "@/lib/api/presets"
import i18n from "@/lib/i18n"
import { mockPlan, mockPlate, mockSession } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/plates", () => ({ createPlate: vi.fn() }))
vi.mock("@/lib/api/presets", () => ({
  getPresets: vi.fn(),
  applyPreset: vi.fn(),
}))
vi.mock("@/lib/api/availability", () => ({
  setAvailabilityOverride: vi.fn(),
  clearAvailabilityOverride: vi.fn(),
  getAvailabilityRange: vi.fn(),
}))
vi.mock("@/lib/api/plan", () => ({ restorePlan: vi.fn() }))
const toastMock = vi.hoisted(() =>
  Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn(), warning: vi.fn() })
)
vi.mock("@/lib/toast", () => ({ toast: toastMock, toastError: vi.fn() }))

import { setAvailabilityOverride } from "@/lib/api/availability"
import { createPlate } from "@/lib/api/plates"
import { applyPreset, getPresets } from "@/lib/api/presets"

import { CellSheet } from "./CellSheet"

/** AC-1 with 2 spare portions (cooked 8, eats 6, use by Wed 4 Nov). */
const spare: Plan = {
  ...mockPlan,
  sessions: [{ ...mockSession, portions: 8, surplus: 2 }],
}

function cell(date: string, slotId: number, plan: Plan = spare): PlanCell {
  return plan.cells.find((c) => c.date === date && c.slot_id === slotId)!
}

const oats: Preset = {
  id: 3,
  name: "Oats",
  tags: [],
  plates: [],
  created_at: "",
  updated_at: "",
}

const applied: ApplyResult = {
  created_plate_ids: [40],
  skipped_occupied: [],
  skipped_unavailable: [],
  skipped_no_slot: [],
  removed: { plates: [], sessions: [] },
  created_overrides: [],
}

async function renderCell(c: PlanCell, plan: Plan = spare) {
  const onPick = vi.fn()
  const onClose = vi.fn()
  renderWithRouter(
    <CellSheet cell={c} plan={plan} onPick={onPick} onClose={onClose} />
  )
  return { onPick, onClose }
}

beforeEach(async () => {
  vi.clearAllMocks()
  await i18n.changeLanguage("en")
  vi.mocked(getPresets).mockResolvedValue({
    items: [oats],
    total: 1,
    known_tags: [],
  })
  vi.mocked(applyPreset).mockResolvedValue(applied)
  vi.mocked(createPlate).mockResolvedValue(mockPlate())
  vi.mocked(setAvailabilityOverride).mockResolvedValue({
    cell: { ...mockPlan.cells[0], mode: "none" },
    removed: null,
  })
})

describe("CellSheet on an empty cell", () => {
  it("opens the food picker for this cell", async () => {
    const { onPick } = await renderCell(cell("2026-11-03", 2))
    await userEvent.click(
      await screen.findByRole("button", { name: "Add food" })
    )
    expect(onPick).toHaveBeenCalledWith({
      kind: "create",
      date: "2026-11-03",
      slotId: 2,
    })
  })

  // F4-AC5: Tuesday lunch is within use-by and eats from the batch.
  it("eats a leftover from a batch with spare portions", async () => {
    const { onClose } = await renderCell(cell("2026-11-03", 2))
    const leftover = await screen.findByTestId("cell-leftover")
    expect(leftover).toHaveTextContent("Chili from Sun, 2 spare")
    await userEvent.click(leftover)
    await waitFor(() =>
      expect(createPlate).toHaveBeenCalledWith({
        date: "2026-11-03",
        slot_id: 2,
        components: [{ food_id: 42, portions: 1, cook_session_id: 7 }],
      })
    )
    await waitFor(() => expect(onClose).toHaveBeenCalled())
  })

  it("lists no batch past its use-by or before its cook cell", async () => {
    await renderCell(cell("2026-11-05", 2))
    await screen.findByRole("button", { name: "Add food" })
    expect(screen.queryByTestId("cell-leftover")).not.toBeInTheDocument()

    await renderCell(cell("2026-11-01", 2))
    expect(screen.queryByTestId("cell-leftover")).not.toBeInTheDocument()
  })

  it("lists no batch without spare portions", async () => {
    await renderCell(cell("2026-11-03", 2, mockPlan), mockPlan)
    await screen.findByRole("button", { name: "Add food" })
    expect(screen.queryByTestId("cell-leftover")).not.toBeInTheDocument()
  })

  it("applies a routine to this meal only", async () => {
    const { onClose } = await renderCell(cell("2026-11-03", 1))
    await userEvent.click(await screen.findByRole("button", { name: "Oats" }))
    await waitFor(() =>
      expect(applyPreset).toHaveBeenCalledWith(3, {
        target_date: "2026-11-03",
        on_conflict: "skip",
        slot_ids_filter: [1],
      })
    )
    expect(getPresets).toHaveBeenCalledWith(
      expect.objectContaining({ slot_ids: [1] })
    )
    await waitFor(() => expect(onClose).toHaveBeenCalled())
    expect(toastMock.success).toHaveBeenCalled()
  })

  it("marks the meal No plate", async () => {
    const { onClose } = await renderCell(cell("2026-11-03", 1))
    await userEvent.click(
      await screen.findByRole("button", { name: "Mark as No plate" })
    )
    await waitFor(() =>
      expect(setAvailabilityOverride).toHaveBeenCalledWith("2026-11-03", 1, {
        mode: "none",
        note: null,
        remove_plate: false,
      })
    )
    expect(onClose).toHaveBeenCalled()
  })
})

describe("CellSheet on a No plate cell", () => {
  const none: PlanCell = {
    ...cell("2026-11-04", 3),
    mode: "none",
    source: "default",
    note: null,
  }

  it("plans the meal", async () => {
    await renderCell(none)
    await userEvent.click(
      await screen.findByRole("button", { name: "Plan this meal" })
    )
    await waitFor(() =>
      expect(setAvailabilityOverride).toHaveBeenCalledWith("2026-11-04", 3, {
        mode: "plan",
      })
    )
    expect(
      screen.queryByRole("button", { name: "Add food" })
    ).not.toBeInTheDocument()
  })

  it("edits the note", async () => {
    await renderCell(none)
    await userEvent.type(await screen.findByTestId("cell-note"), "Eating out")
    await userEvent.click(screen.getByRole("button", { name: "Save note" }))
    await waitFor(() =>
      expect(setAvailabilityOverride).toHaveBeenCalledWith("2026-11-04", 3, {
        mode: "none",
        note: "Eating out",
      })
    )
  })
})
