import { beforeEach, describe, expect, it, vi } from "vitest"
import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import type { Plan, PlanCell } from "@/lib/api/plan"
import { renderWithRouter } from "@/test/render"
import {
  mockChili,
  mockPlan,
  mockPlate,
  mockPlateComponent,
  mockProfile,
} from "@/test/fixtures"

vi.mock("@/lib/api/plan", () => ({ getPlan: vi.fn(), restorePlan: vi.fn() }))
vi.mock("@/lib/api/plates", () => ({
  createPlate: vi.fn(),
  deletePlate: vi.fn(),
}))
vi.mock("@/lib/api/profile", () => ({ getProfile: vi.fn() }))

import { getPlan, restorePlan } from "@/lib/api/plan"
import { createPlate, deletePlate } from "@/lib/api/plates"
import { getProfile } from "@/lib/api/profile"

import { AddToPlanSheet } from "./AddToPlanSheet"

function cell(date: string, slot_id: number, patch: Partial<PlanCell> = {}) {
  return {
    date,
    slot_id,
    mode: "plan",
    source: "default",
    preset_id: null,
    preset_name: null,
    note: null,
    plate: null,
    ...patch,
  } satisfies PlanCell
}

/** Today (Sat 31 Oct): breakfast No plate, lunch a routine, dinner taken.
 *  Sunday breakfast is the first free plan cell. */
const current: Plan = {
  ...mockPlan,
  cells: [
    cell("2026-10-31", 1, { mode: "none" }),
    cell("2026-10-31", 2, { mode: "routine", preset_id: 1 }),
    cell("2026-10-31", 3, { plate: mockPlate() }),
    cell("2026-11-01", 3),
    cell("2026-11-01", 1),
  ],
}

const next: Plan = {
  ...mockPlan,
  window: {
    ...mockPlan.window,
    start: "2026-11-07",
    end: "2026-11-13",
    is_current: false,
    prev_start: "2026-10-31",
    next_start: "2026-11-14",
  },
  cells: [cell("2026-11-07", 2)],
}

function renderSheet() {
  return renderWithRouter(
    <AddToPlanSheet recipe={mockChili} open onOpenChange={vi.fn()} />
  )
}

describe("AddToPlanSheet", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getPlan).mockImplementation(async (start) =>
      start === "2026-11-07" ? next : current
    )
    vi.mocked(getProfile).mockResolvedValue({
      ...mockProfile,
      household_portions: 2,
    })
    vi.mocked(createPlate).mockResolvedValue(mockPlate())
  })

  it("F6-AC3: preselects the first empty plan cell and skips none/routine/occupied cells", async () => {
    renderSheet()

    const first = await screen.findByTestId("add-to-plan-cell-2026-11-01|1")
    expect(first).toBeChecked()
    expect(first).toHaveAccessibleName("Sunday, November 1, Breakfast")
    const radios = screen.getAllByRole("radio")
    expect(radios.map((r) => (r as HTMLInputElement).value)).toEqual([
      "2026-11-01|1",
      "2026-11-01|3",
      "2026-11-07|2",
    ])
    expect(getPlan).toHaveBeenCalledWith("2026-11-07")
  })

  it("Add creates a plate with one composed component on the picked cell", async () => {
    const user = userEvent.setup()
    renderSheet()

    await user.click(await screen.findByTestId("add-to-plan-cell-2026-11-07|2"))
    expect(await screen.findByTestId("add-to-plan-eaters")).toHaveTextContent(
      "2"
    )
    await user.click(screen.getByTestId("add-to-plan-submit"))

    await vi.waitFor(() =>
      expect(createPlate).toHaveBeenCalledWith({
        date: "2026-11-07",
        slot_id: 2,
        eaters: 2,
        components: [{ food_id: mockChili.id, portions: 1 }],
      })
    )
  })

  it("with a next-week anchor, offers today's window and the anchored one", async () => {
    vi.mocked(getPlan).mockImplementation(async (start) =>
      start === "2026-10-31"
        ? current
        : { ...next, window: { ...next.window, today: "2026-10-31" } }
    )
    renderSheet()

    expect(
      await screen.findByTestId("add-to-plan-cell-2026-11-01|1")
    ).toBeChecked()
    expect(getPlan).toHaveBeenCalledWith("2026-10-31")
    expect(screen.getAllByRole("radio")).toHaveLength(3)
  })

  it("leaves eaters to the server while the profile loads", async () => {
    vi.mocked(getProfile).mockReturnValue(new Promise(() => {}))
    const user = userEvent.setup()
    renderSheet()

    await user.click(await screen.findByTestId("add-to-plan-submit"))
    await vi.waitFor(() => expect(createPlate).toHaveBeenCalled())
    expect(vi.mocked(createPlate).mock.calls[0][0].eaters).toBeUndefined()
  })

  it("says so when both windows are full", async () => {
    vi.mocked(getPlan).mockResolvedValue({ ...current, cells: [] })
    renderSheet()

    expect(await screen.findByTestId("add-to-plan-empty")).toHaveTextContent(
      "No free meals"
    )
  })

  // DoD should-fix 2: after a generate every cell holds a draft. Those cells
  // are offered too, marked as replacing the draft; Add deletes it first.
  describe("over draft plates", () => {
    const draft = mockPlate({
      id: 77,
      date: "2026-11-01",
      slot_id: 3,
      state: "draft",
      components: [
        mockPlateComponent({ id: 5, plate_id: 77, food_name: "Lentil soup" }),
      ],
    })
    const drafted: Plan = {
      ...current,
      cells: [
        cell("2026-10-31", 3, { plate: mockPlate() }),
        cell("2026-11-01", 3, { plate: draft }),
      ],
    }
    const removed = { plates: [], sessions: [] }

    beforeEach(() => {
      vi.mocked(getPlan).mockImplementation(async (start) =>
        start === "2026-11-07" ? { ...next, cells: [] } : drafted
      )
      vi.mocked(deletePlate).mockResolvedValue({ removed })
    })

    it("offers a draft cell, labelled as replacing the draft", async () => {
      renderSheet()
      const radio = await screen.findByTestId("add-to-plan-cell-2026-11-01|3")
      expect(radio).toBeChecked()
      expect(radio).toHaveAccessibleName(
        "Sunday, November 1, Dinner. Replaces the draft Lentil soup"
      )
      expect(screen.getAllByRole("radio")).toHaveLength(1)
      expect(screen.queryByTestId("add-to-plan-empty")).toBeNull()
    })

    it("Add deletes the draft, then creates the plate", async () => {
      const user = userEvent.setup()
      renderSheet()
      await user.click(await screen.findByTestId("add-to-plan-submit"))
      await vi.waitFor(() => expect(createPlate).toHaveBeenCalled())
      expect(deletePlate).toHaveBeenCalledWith(77)
      expect(vi.mocked(deletePlate).mock.invocationCallOrder[0]).toBeLessThan(
        vi.mocked(createPlate).mock.invocationCallOrder[0]
      )
      expect(vi.mocked(createPlate).mock.calls[0][0]).toMatchObject({
        date: "2026-11-01",
        slot_id: 3,
      })
      expect(restorePlan).not.toHaveBeenCalled()
    })

    it("restores the draft when the create fails", async () => {
      vi.mocked(createPlate).mockRejectedValue(new Error("offline"))
      const user = userEvent.setup()
      renderSheet()
      await user.click(await screen.findByTestId("add-to-plan-submit"))
      await vi.waitFor(() => expect(restorePlan).toHaveBeenCalledWith(removed))
    })

    it("prefers a free cell over a draft one", async () => {
      vi.mocked(getPlan).mockImplementation(async (start) =>
        start === "2026-11-07" ? next : drafted
      )
      renderSheet()
      expect(
        await screen.findByTestId("add-to-plan-cell-2026-11-07|2")
      ).toBeChecked()
    })
  })
})
