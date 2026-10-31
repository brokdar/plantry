import { beforeEach, describe, expect, it, vi } from "vitest"
import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import type { Plan, PlanCell } from "@/lib/api/plan"
import { renderWithRouter } from "@/test/render"
import { mockChili, mockPlan, mockPlate, mockProfile } from "@/test/fixtures"

vi.mock("@/lib/api/plan", () => ({ getPlan: vi.fn() }))
vi.mock("@/lib/api/plates", () => ({ createPlate: vi.fn() }))
vi.mock("@/lib/api/profile", () => ({ getProfile: vi.fn() }))

import { getPlan } from "@/lib/api/plan"
import { createPlate } from "@/lib/api/plates"
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

  it("says so when both windows are full", async () => {
    vi.mocked(getPlan).mockResolvedValue({ ...current, cells: [] })
    renderSheet()

    expect(await screen.findByTestId("add-to-plan-empty")).toHaveTextContent(
      "No free meals"
    )
  })
})
