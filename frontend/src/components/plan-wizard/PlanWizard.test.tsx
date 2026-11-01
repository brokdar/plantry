import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import {
  mockAvailabilityDefaults,
  mockPlan,
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
}))
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
  getAvailabilityDefaults,
  setAvailabilityDefaults,
  setAvailabilityOverride,
} from "@/lib/api/availability"
import { generatePlan, getPlanRun } from "@/lib/api/generation"
import { getPlan } from "@/lib/api/plan"
import { getPresets } from "@/lib/api/presets"
import { getProfile } from "@/lib/api/profile"
import { listSettings } from "@/lib/api/settings"

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

  it("a No plate cell that holds a plate removes the plate", async () => {
    const user = await openWizard()
    // Sunday dinner cooks the Chili in mockPlan.
    await user.click(cell(/^Sunday, November 1 dinner/))
    await user.click(screen.getByRole("button", { name: "Next" }))
    await screen.findByRole("heading", { name: "Anything special this week?" })
    expect(setAvailabilityOverride).toHaveBeenCalledWith("2026-11-01", 3, {
      mode: "none",
      preset_id: null,
      remove_plate: true,
    })
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
    await user.click(screen.getByRole("button", { name: /^Monday.* actions$/ }))
    await user.click(screen.getByRole("menuitem", { name: "Away all day" }))
    for (const slot of ["breakfast", "lunch", "dinner"]) {
      expect(
        cell(new RegExp(`^Monday, November 2 ${slot}, No plate$`))
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

  it("Skip generates without the note; the toggle follows plan.explore_per_week", async () => {
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
    await user.click(screen.getByRole("button", { name: "Skip" }))
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
    await user.click(await screen.findByRole("button", { name: "Skip" }))
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
    })
    const user = await toGenerating()
    expect(
      await screen.findByText(
        "The assistant didn't respond. Try again, or plan without the assistant."
      )
    ).toBeInTheDocument()

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
    vi.mocked(getPlanRun).mockResolvedValue(mockRun("error"))
    const user = await toGenerating()
    expect(
      await screen.findByText("The plan couldn't be made. Try again.")
    ).toBeInTheDocument()
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
})
