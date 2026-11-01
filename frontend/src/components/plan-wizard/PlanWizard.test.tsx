import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ApiError } from "@/lib/api/client"
import type { Plan } from "@/lib/api/plan"
import {
  mockAvailabilityDefaults,
  mockDraftPlan,
  mockPlan,
  mockRemovedBatch,
  mockProfile,
  mockRun,
  mockSettingsList,
} from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

const navigateMock = vi.hoisted(() => vi.fn())
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useNavigate: () => navigateMock,
}))

vi.mock("@/lib/api/plan", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/plan")>()),
  getPlan: vi.fn(),
  restorePlan: vi.fn(),
}))
// The Undo toast: its action is clicked by hand.
const toastMock = vi.hoisted(() =>
  Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn() })
)
vi.mock("@/lib/toast", () => ({ toast: toastMock, toastError: vi.fn() }))
vi.mock("@/lib/api/settings", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/settings")>()),
  listSettings: vi.fn(),
}))
vi.mock("@/lib/api/profile", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/profile")>()),
  getProfile: vi.fn(),
}))
vi.mock("@/lib/api/presets", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/presets")>()),
  getPresets: vi.fn(),
}))
vi.mock("@/lib/api/availability", () => ({
  getAvailabilityDefaults: vi.fn(),
  setAvailabilityDefaults: vi.fn(),
  setAvailabilityOverride: vi.fn(),
  clearAvailabilityOverride: vi.fn(),
  getAvailabilityRange: vi.fn(),
}))
vi.mock("@/lib/api/generation", () => ({
  generatePlan: vi.fn(),
  regeneratePlan: vi.fn(),
  getPlanRun: vi.fn(),
}))

import {
  clearAvailabilityOverride,
  getAvailabilityDefaults,
  getAvailabilityRange,
  setAvailabilityDefaults,
  setAvailabilityOverride,
} from "@/lib/api/availability"
import { generatePlan, getPlanRun } from "@/lib/api/generation"
import { getPlan, restorePlan } from "@/lib/api/plan"
import { getPresets } from "@/lib/api/presets"
import { getProfile } from "@/lib/api/profile"
import { listSettings } from "@/lib/api/settings"
import i18n from "@/lib/i18n"
import { toastError } from "@/lib/toast"

import { PlanWizard } from "./PlanWizard"

const START = mockPlan.window.start

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(getPlan).mockResolvedValue(mockPlan)
  vi.mocked(listSettings).mockResolvedValue(mockSettingsList())
  vi.mocked(getProfile).mockResolvedValue(mockProfile)
  vi.mocked(getPresets).mockResolvedValue({ items: [], total: 0 } as never)
  vi.mocked(getAvailabilityDefaults).mockResolvedValue({
    items: mockAvailabilityDefaults,
  })
  vi.mocked(setAvailabilityDefaults).mockImplementation(async (items) => ({
    items,
  }))
  vi.mocked(setAvailabilityOverride).mockResolvedValue({
    cell: {} as never,
    removed: null,
  })
  vi.mocked(generatePlan).mockResolvedValue({ run_id: 12 })
  vi.mocked(getPlanRun).mockResolvedValue(mockRun("running", "choosing"))
})

/** The grid cell button for a date (en "Thursday, November 5") and slot. */
function cell(name: RegExp) {
  return screen.getByRole("button", { name })
}

async function openWizard() {
  const user = userEvent.setup()
  renderWithRouter(<PlanWizard start={START} />)
  await screen.findByRole("heading", { name: "Who's home?" })
  return user
}

describe("PlanWizard", () => {
  it("step 1 shows the window prefilled; Next writes the changed cells as overrides", async () => {
    const user = await openWizard()
    const grid = screen.getByRole("grid", { name: /Who's home/ })
    // Seven dates × three meals, all Plan as the cells resolve.
    expect(within(grid).getAllByRole("gridcell")).toHaveLength(21)
    expect(cell(/^Thursday, November 5 dinner, Plan$/)).toBeInTheDocument()

    await user.click(cell(/^Thursday, November 5 dinner/))
    await user.click(cell(/^Friday, November 6 lunch/))
    expect(cell(/^Thursday, November 5 dinner, No plate$/)).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Next" }))
    await screen.findByRole("heading", { name: "Anything special this week?" })
    expect(setAvailabilityOverride).toHaveBeenCalledTimes(2)
    expect(setAvailabilityOverride).toHaveBeenCalledWith("2026-11-05", 3, {
      mode: "none",
      preset_id: null,
      remove_plate: false,
    })
    expect(setAvailabilityOverride).toHaveBeenCalledWith("2026-11-06", 2, {
      mode: "none",
      preset_id: null,
      remove_plate: false,
    })
    expect(setAvailabilityDefaults).not.toHaveBeenCalled()
  })

  it("No plate on a confirmed plate asks first, then removes it with Undo", async () => {
    vi.mocked(setAvailabilityOverride).mockResolvedValue({
      cell: {} as never,
      removed: mockRemovedBatch.removed,
    })
    const user = await openWizard()
    // Sunday dinner cooks the confirmed Chili in mockPlan.
    await user.click(cell(/^Sunday, November 1 dinner/))
    await user.click(screen.getByRole("button", { name: "Next" }))
    const dialog = await screen.findByTestId("wizard-confirm-removal")
    expect(dialog).toHaveTextContent("Remove a confirmed plate?")
    expect(dialog).toHaveTextContent("Sunday dinner: Chili")

    // Keep them: nothing is written, the wizard stays on step 1.
    await user.click(within(dialog).getByRole("button", { name: "Keep it" }))
    expect(screen.queryByTestId("wizard-confirm-removal")).toBeNull()
    expect(setAvailabilityOverride).not.toHaveBeenCalled()

    await user.click(screen.getByRole("button", { name: "Next" }))
    await user.click(
      within(await screen.findByTestId("wizard-confirm-removal")).getByRole(
        "button",
        { name: "Remove" }
      )
    )
    await screen.findByRole("heading", { name: "Anything special this week?" })
    expect(setAvailabilityOverride).toHaveBeenCalledWith("2026-11-01", 3, {
      mode: "none",
      preset_id: null,
      remove_plate: true,
    })

    // Undo gives the cell back to its default and restores the plate.
    expect(toastMock).toHaveBeenCalledWith(
      "1 plate removed",
      expect.objectContaining({ action: expect.anything() })
    )
    const { action } = toastMock.mock.calls[0]![1] as {
      action: { onClick: () => void }
    }
    action.onClick()
    await waitFor(() =>
      expect(restorePlan).toHaveBeenCalledWith(mockRemovedBatch.removed)
    )
    expect(clearAvailabilityOverride).toHaveBeenCalledWith("2026-11-01", 3)
  })

  it("a failed Undo marks the cell No plate again and names the conflict", async () => {
    vi.mocked(setAvailabilityOverride).mockResolvedValue({
      cell: {} as never,
      removed: mockRemovedBatch.removed,
    })
    vi.mocked(restorePlan).mockRejectedValue(
      new ApiError(409, "error.plan.cell_occupied")
    )
    const user = await openWizard()
    await user.click(cell(/^Sunday, November 1 dinner/))
    await user.click(screen.getByRole("button", { name: "Next" }))
    await user.click(
      within(await screen.findByTestId("wizard-confirm-removal")).getByRole(
        "button",
        { name: "Remove" }
      )
    )
    await screen.findByRole("heading", { name: "Anything special this week?" })
    const { action } = toastMock.mock.calls[0]![1] as {
      action: { onClick: () => void }
    }
    vi.mocked(setAvailabilityOverride).mockClear()
    action.onClick()
    await waitFor(() =>
      expect(toastMock.error).toHaveBeenCalledWith(
        "Can't undo: that meal was changed in the meantime."
      )
    )
    expect(setAvailabilityOverride).toHaveBeenCalledWith("2026-11-01", 3, {
      mode: "none",
      note: null,
    })
  })

  it("plates of one batch are removed without Undo", async () => {
    vi.mocked(setAvailabilityOverride).mockResolvedValue({
      cell: {} as never,
      removed: mockRemovedBatch.removed,
    })
    const user = await openWizard()
    // Sunday's Chili batch feeds Monday lunch: one cook session.
    await user.click(cell(/^Sunday, November 1 dinner/))
    await user.click(cell(/^Monday, November 2 lunch/))
    await user.click(screen.getByRole("button", { name: "Next" }))
    const dialog = await screen.findByTestId("wizard-confirm-removal")
    expect(dialog).toHaveTextContent("Remove 2 confirmed plates?")
    await user.click(within(dialog).getByRole("button", { name: "Remove" }))
    await screen.findByRole("heading", { name: "Anything special this week?" })
    expect(toastMock).toHaveBeenCalledWith(
      "2 plates removed. They share a batch, so this can't be undone."
    )
  })

  it("unticking Make this my new default puts the replaced defaults back", async () => {
    const user = await openWizard()
    await user.click(cell(/^Thursday, November 5 dinner/))
    await user.click(
      screen.getByRole("checkbox", { name: "Make this my new default" })
    )
    await user.click(screen.getByRole("button", { name: "Next" }))
    await screen.findByRole("heading", { name: "Anything special this week?" })
    expect(setAvailabilityDefaults).toHaveBeenCalledTimes(1)

    await user.click(screen.getByRole("button", { name: "Back" }))
    await user.click(
      await screen.findByRole("checkbox", { name: "Make this my new default" })
    )
    await user.click(screen.getByRole("button", { name: "Next" }))
    await screen.findByRole("heading", { name: "Anything special this week?" })
    expect(setAvailabilityDefaults).toHaveBeenCalledTimes(2)
    expect(setAvailabilityDefaults).toHaveBeenLastCalledWith(
      mockAvailabilityDefaults
    )
    expect(setAvailabilityOverride).toHaveBeenCalledWith("2026-11-05", 3, {
      mode: "none",
      preset_id: null,
      remove_plate: false,
    })
  })

  it("a draft plate is removed without asking", async () => {
    vi.mocked(getPlan).mockResolvedValue({
      ...mockPlan,
      cells: mockPlan.cells.map((c) =>
        c.plate ? { ...c, plate: { ...c.plate, state: "draft" } } : c
      ),
    } as Plan)
    const user = await openWizard()
    await user.click(cell(/^Sunday, November 1 dinner/))
    await user.click(screen.getByRole("button", { name: "Next" }))
    await screen.findByRole("heading", { name: "Anything special this week?" })
    expect(screen.queryByTestId("wizard-confirm-removal")).toBeNull()
    expect(setAvailabilityOverride).toHaveBeenCalledWith("2026-11-01", 3, {
      mode: "none",
      preset_id: null,
      remove_plate: true,
    })
  })

  it("back to Plan on an override clears it when the default plans the meal", async () => {
    vi.mocked(getPlan).mockResolvedValue({
      ...mockPlan,
      cells: mockPlan.cells.map((c) =>
        c.date === "2026-11-05" && c.slot_id === 3
          ? { ...c, mode: "none", source: "override" }
          : c
      ),
    } as Plan)
    vi.mocked(getAvailabilityRange).mockResolvedValue({
      items: [
        {
          date: "2026-11-05",
          slot_id: 3,
          mode: "plan",
          source: "default",
          preset_id: null,
          preset_name: null,
          note: null,
        },
      ],
    })
    const user = await openWizard()
    await user.click(cell(/^Thursday, November 5 dinner, No plate$/))
    await user.click(screen.getByRole("button", { name: "Next" }))
    await screen.findByRole("heading", { name: "Anything special this week?" })
    expect(clearAvailabilityOverride).toHaveBeenCalledWith("2026-11-05", 3)
    expect(setAvailabilityOverride).not.toHaveBeenCalled()
  })

  it("Back, then Make this my new default, writes the same changes as defaults", async () => {
    const user = await openWizard()
    await user.click(cell(/^Thursday, November 5 dinner/))
    await user.click(screen.getByRole("button", { name: "Next" }))
    await screen.findByRole("heading", { name: "Anything special this week?" })
    expect(setAvailabilityOverride).toHaveBeenCalledTimes(1)

    await user.click(screen.getByRole("button", { name: "Back" }))
    await user.click(
      await screen.findByRole("checkbox", { name: "Make this my new default" })
    )
    await user.click(screen.getByRole("button", { name: "Next" }))
    await screen.findByRole("heading", { name: "Anything special this week?" })
    expect(setAvailabilityDefaults).toHaveBeenCalledTimes(1)
    expect(vi.mocked(setAvailabilityDefaults).mock.calls[0]![0]).toContainEqual(
      { weekday: 3, slot_id: 3, mode: "none", preset_id: null }
    )
  })

  it("F3-AC2: Make this my new default writes the defaults, not overrides", async () => {
    const user = await openWizard()
    await user.click(cell(/^Thursday, November 5 dinner/))
    await user.click(
      screen.getByRole("checkbox", { name: "Make this my new default" })
    )
    await user.click(screen.getByRole("button", { name: "Next" }))
    await screen.findByRole("heading", { name: "Anything special this week?" })

    expect(setAvailabilityOverride).not.toHaveBeenCalled()
    expect(setAvailabilityDefaults).toHaveBeenCalledTimes(1)
    const items = vi.mocked(setAvailabilityDefaults).mock.calls[0]![0]
    // Thursday is weekday 3; every weekday × slot is sent.
    expect(items).toHaveLength(21)
    expect(items).toContainEqual({
      weekday: 3,
      slot_id: 3,
      mode: "none",
      preset_id: null,
    })
    // Every other default stays as stored.
    expect(items.filter((i) => !(i.weekday === 3 && i.slot_id === 3))).toEqual(
      mockAvailabilityDefaults.filter(
        (i) => !(i.weekday === 3 && i.slot_id === 3)
      )
    )
  })

  it("without changes Next writes nothing", async () => {
    const user = await openWizard()
    await user.click(screen.getByRole("button", { name: "Next" }))
    await screen.findByRole("heading", { name: "Anything special this week?" })
    expect(setAvailabilityOverride).not.toHaveBeenCalled()
    expect(setAvailabilityDefaults).not.toHaveBeenCalled()
  })

  it("the day menu marks a day away and adds guests; Generate sends the note, guests and the new-recipe toggle", async () => {
    const user = await openWizard()
    await user.click(
      screen.getByRole("button", { name: /^Tuesday.* actions$/ })
    )
    await user.click(screen.getByRole("menuitem", { name: "Away all day" }))
    for (const slot of ["breakfast", "lunch", "dinner"]) {
      expect(
        cell(new RegExp(`^Tuesday, November 3 ${slot}, No plate$`))
      ).toBeInTheDocument()
    }

    await user.click(screen.getByRole("button", { name: /^Friday.* actions$/ }))
    await user.click(screen.getByRole("menuitem", { name: "Guests" }))
    const guests = screen.getByTestId("wizard-guests-2026-11-06")
    // Household of 1, plus one guest; one more press makes it 2½.
    expect(guests).toHaveTextContent("2")
    await user.click(within(guests).getByRole("button", { name: /^Increase/ }))

    await user.click(screen.getByRole("button", { name: "Next" }))
    await screen.findByRole("heading", { name: "Anything special this week?" })
    // plan.explore_per_week = 0: the toggle starts off.
    const toggle = screen.getByRole("switch", { name: "Include a new recipe" })
    expect(toggle).not.toBeChecked()
    await user.click(toggle)
    await user.type(
      screen.getByRole("textbox", { name: "Note for this week" }),
      "  something light  "
    )
    await user.click(screen.getByRole("button", { name: "Generate plan" }))

    await screen.findByRole("heading", { name: "Planning your week" })
    expect(generatePlan).toHaveBeenCalledWith({
      start: START,
      instruction: "something light",
      eaters_by_date: { "2026-11-06": 2.5 },
      include_new_recipe: true,
      chooser: "heuristic",
    })
  })

  it("Generate without a note leaves the note out; the toggle follows plan.explore_per_week", async () => {
    vi.mocked(listSettings).mockResolvedValue(
      mockSettingsList([{ key: "plan.explore_per_week", value: "1" }])
    )
    const user = await openWizard()
    await user.click(screen.getByRole("button", { name: "Next" }))
    await user.type(
      await screen.findByRole("textbox", { name: "Note for this week" }),
      "ignored"
    )
    expect(
      screen.getByRole("switch", { name: "Include a new recipe" })
    ).toBeChecked()
    await user.click(
      screen.getByRole("button", { name: "Generate without a note" })
    )
    await screen.findByRole("heading", { name: "Planning your week" })
    expect(generatePlan).toHaveBeenCalledWith({
      start: START,
      instruction: undefined,
      eaters_by_date: undefined,
      include_new_recipe: true,
      chooser: "heuristic",
    })
  })

  async function toGenerating() {
    const user = await openWizard()
    await user.click(screen.getByRole("button", { name: "Next" }))
    await user.click(
      await screen.findByRole("button", { name: "Generate without a note" })
    )
    await screen.findByRole("heading", { name: "Planning your week" })
    return user
  }

  it("step 3 lists the stages and opens the board with the run when it is done", async () => {
    vi.mocked(getPlanRun)
      .mockResolvedValueOnce(mockRun("running", "checking"))
      .mockResolvedValue(mockRun("ok"))
    await toGenerating()
    const steps = await screen.findAllByRole("listitem")
    expect(steps.map((s) => s.textContent)).toEqual([
      expect.stringMatching(/^Reading your week/),
      expect.stringMatching(/^Choosing recipes/),
      expect.stringMatching(/^Checking batches and keep days/),
      expect.stringMatching(/^Saving draft/),
    ])
    await waitFor(() =>
      expect(navigateMock).toHaveBeenCalledWith({
        to: "/plan",
        search: { start: START, run: 12 },
        viewTransition: true,
      })
    )
  })

  it("with an assistant configured the server picks the chooser", async () => {
    vi.mocked(listSettings).mockResolvedValue(
      mockSettingsList([{ key: "ai.provider", value: "anthropic" }])
    )
    await toGenerating()
    expect(vi.mocked(generatePlan).mock.calls[0]![0]).not.toHaveProperty(
      "chooser"
    )
  })

  it("F3-AC3: an assistant failure offers Try again and Plan without the assistant", async () => {
    vi.mocked(listSettings).mockResolvedValue(
      mockSettingsList([{ key: "ai.provider", value: "anthropic" }])
    )
    vi.mocked(getPlanRun).mockResolvedValue({
      ...mockRun("error"),
      chooser: "llm",
      error_key: "error.ai.invalid_plan",
    })
    const user = await toGenerating()
    // A neutral headline over the run's own error, not "didn't respond".
    const alert = await screen.findByRole("alert")
    expect(alert).toHaveTextContent("Planning didn't work.")
    expect(alert).toHaveTextContent(i18n.t("error.ai.invalid_plan"))
    expect(alert).not.toHaveTextContent("didn't respond")

    vi.mocked(generatePlan).mockResolvedValue({ run_id: 13 })
    vi.mocked(getPlanRun).mockResolvedValue({ ...mockRun("ok"), id: 13 })
    await user.click(
      screen.getByRole("button", { name: "Plan without the assistant" })
    )
    expect(generatePlan).toHaveBeenLastCalledWith({
      start: START,
      instruction: undefined,
      eaters_by_date: undefined,
      include_new_recipe: false,
      chooser: "heuristic",
    })
    await waitFor(() =>
      expect(navigateMock).toHaveBeenCalledWith({
        to: "/plan",
        search: { start: START, run: 13 },
        viewTransition: true,
      })
    )
  })

  it("a heuristic failure offers only Try again, which repeats the request", async () => {
    vi.mocked(getPlanRun).mockResolvedValue({
      ...mockRun("error"),
      error_key: null,
    })
    const user = await toGenerating()
    // Without an error key the chooser's advice stands in.
    const alert = await screen.findByRole("alert")
    expect(alert).toHaveTextContent("Planning didn't work.")
    expect(alert).toHaveTextContent("The plan couldn't be made. Try again.")
    expect(
      screen.queryByRole("button", { name: "Plan without the assistant" })
    ).toBeNull()
    await user.click(screen.getByRole("button", { name: "Try again" }))
    expect(generatePlan).toHaveBeenCalledTimes(2)
    expect(vi.mocked(generatePlan).mock.calls[1]![0]).toEqual(
      vi.mocked(generatePlan).mock.calls[0]![0]
    )
  })

  it("closing goes back to the board of the window", async () => {
    const user = await openWizard()
    await user.keyboard("{Escape}")
    expect(navigateMock).toHaveBeenCalledWith({
      to: "/plan",
      search: { start: START, run: undefined },
    })
  })

  it("Escape with unsaved edits asks first; Keep editing stays, Discard closes", async () => {
    const user = await openWizard()
    await user.click(cell(/^Thursday, November 5 dinner/))
    await user.keyboard("{Escape}")
    const dialog = await screen.findByTestId("wizard-discard")
    expect(dialog).toHaveTextContent("Discard your changes?")
    expect(navigateMock).not.toHaveBeenCalled()

    await user.click(
      within(dialog).getByRole("button", { name: "Keep editing" })
    )
    expect(screen.queryByTestId("wizard-discard")).toBeNull()
    expect(cell(/^Thursday, November 5 dinner, No plate$/)).toBeInTheDocument()

    await user.keyboard("{Escape}")
    await user.click(
      within(await screen.findByTestId("wizard-discard")).getByRole("button", {
        name: "Discard",
      })
    )
    expect(navigateMock).toHaveBeenCalledWith({
      to: "/plan",
      search: { start: START, run: undefined },
    })
    expect(setAvailabilityOverride).not.toHaveBeenCalled()
  })

  it("warns that generating replaces the window's draft", async () => {
    vi.mocked(getPlan).mockResolvedValue(mockDraftPlan)
    await openWizard()
    expect(screen.getByTestId("wizard-replaces-draft")).toHaveTextContent(
      "This week has a draft of 3 plates. Generating replaces it; confirmed plates stay."
    )
  })

  it("has no draft warning without drafts", async () => {
    await openWizard()
    expect(screen.queryByTestId("wizard-replaces-draft")).toBeNull()
  })

  it("days before today are shown as past and can't be changed", async () => {
    vi.mocked(getPlan).mockResolvedValue({
      ...mockPlan,
      window: { ...mockPlan.window, today: "2026-11-02" },
    })
    await openWizard()
    expect(cell(/^Saturday, October 31 dinner/)).toBeDisabled()
    expect(cell(/^Sunday, November 1 lunch/)).toBeDisabled()
    expect(
      screen.getByRole("button", { name: /^Sunday.* actions$/ })
    ).toBeDisabled()
    expect(cell(/^Monday, November 2 dinner/)).toBeEnabled()
    expect(screen.getAllByTestId("grid-column-note")).toHaveLength(2)
    expect(screen.getAllByTestId("grid-column-note")[0]).toHaveTextContent(
      "Past"
    )
    // The grid's tab stop is on the first day that can change.
    expect(cell(/^Monday, November 2 breakfast/)).toHaveAttribute(
      "tabindex",
      "0"
    )
  })

  it("Guests moves focus to the new stepper and marks the day", async () => {
    const user = await openWizard()
    await user.click(screen.getByRole("button", { name: /^Friday.* actions$/ }))
    await user.click(screen.getByRole("menuitem", { name: "Guests" }))
    const stepper = screen.getByTestId("wizard-guests-2026-11-06")
    await waitFor(() =>
      expect(stepper).toContainElement(document.activeElement as HTMLElement)
    )
    expect(screen.getByTestId("grid-column-note")).toHaveTextContent("Guests")
  })

  it("a write failing part-way keeps Undo for the plates already removed", async () => {
    vi.mocked(setAvailabilityOverride)
      .mockResolvedValueOnce({
        cell: {} as never,
        removed: mockRemovedBatch.removed,
      })
      .mockRejectedValueOnce(new ApiError(503, "error.network"))
    const user = await openWizard()
    // Sunday dinner's confirmed Chili is removed, then Thursday fails.
    await user.click(cell(/^Sunday, November 1 dinner/))
    await user.click(cell(/^Thursday, November 5 dinner/))
    await user.click(screen.getByRole("button", { name: "Next" }))
    await user.click(
      within(await screen.findByTestId("wizard-confirm-removal")).getByRole(
        "button",
        { name: "Remove" }
      )
    )
    await waitFor(() =>
      expect(toastMock).toHaveBeenCalledWith(
        "1 plate removed",
        expect.objectContaining({ action: expect.anything() })
      )
    )
    expect(toastError).toHaveBeenCalledWith(
      new ApiError(503, "error.network"),
      expect.anything()
    )
    // Still on step 1; Undo restores the removed plate.
    expect(
      screen.getByRole("heading", { name: "Who's home?" })
    ).toBeInTheDocument()
    const { action } = toastMock.mock.calls[0]![1] as {
      action: { onClick: () => void }
    }
    action.onClick()
    await waitFor(() =>
      expect(restorePlan).toHaveBeenCalledWith(mockRemovedBatch.removed)
    )
  })
})
