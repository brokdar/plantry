import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ApiError } from "@/lib/api/client"
import type { TimeSlot, TimeSlotInput } from "@/lib/api/slots"
import i18n from "@/lib/i18n"
import { mockProfile, mockSettingsList } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

const navigate = vi.fn()
vi.mock("@tanstack/react-router", async (orig) => ({
  ...(await orig<typeof import("@tanstack/react-router")>()),
  useNavigate: () => navigate,
}))

vi.mock("@/lib/api/settings", () => ({
  listSettings: vi.fn(),
  setSetting: vi.fn(),
  clearSetting: vi.fn(),
  listAIModels: vi.fn(),
  getSystemInfo: vi.fn(),
}))
vi.mock("@/lib/api/profile", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/profile")>()),
  getProfile: vi.fn(),
  updateProfile: vi.fn(),
}))
vi.mock("@/lib/api/slots", () => ({
  listTimeSlots: vi.fn(),
  createTimeSlot: vi.fn(),
  updateTimeSlot: vi.fn(),
  deleteTimeSlot: vi.fn(),
}))
vi.mock("@/lib/api/availability", () => ({
  getAvailabilityDefaults: vi.fn(),
  setAvailabilityDefaults: vi.fn(),
  getAvailabilityRange: vi.fn(),
  setAvailabilityOverride: vi.fn(),
  clearAvailabilityOverride: vi.fn(),
}))
vi.mock("@/lib/api/presets", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/presets")>()),
  getPresets: vi.fn(),
}))
vi.mock("@/lib/api/setup", () => ({
  getSetupStatus: vi.fn(),
  completeSetup: vi.fn(),
}))

import { setAvailabilityDefaults } from "@/lib/api/availability"
import { getPresets } from "@/lib/api/presets"
import { getProfile, updateProfile, type Profile } from "@/lib/api/profile"
import { listSettings, setSetting } from "@/lib/api/settings"
import { completeSetup } from "@/lib/api/setup"
import { createTimeSlot, listTimeSlots, updateTimeSlot } from "@/lib/api/slots"
import { getAvailabilityDefaults } from "@/lib/api/availability"

import { SetupWizard } from "./SetupWizard"

/** A tiny server: GETs return what the writes stored. */
function serve(
  initialSettings: Record<string, string> = {},
  initialProfile: Partial<Profile> = {}
) {
  let profile: Profile = { ...mockProfile, ...initialProfile }
  const settings = new Map<string, string>(Object.entries(initialSettings))
  const slots: TimeSlot[] = []
  vi.mocked(getProfile).mockImplementation(async () => profile)
  vi.mocked(updateProfile).mockImplementation(async (input) => {
    profile = { ...profile, ...input } as Profile
    return profile
  })
  vi.mocked(listSettings).mockImplementation(async () =>
    mockSettingsList(
      [...settings].map(([key, value]) => ({ key, value, source: "db" }))
    )
  )
  vi.mocked(setSetting).mockImplementation(async (key, value) => {
    settings.set(key, value)
  })
  vi.mocked(listTimeSlots).mockImplementation(async () => ({ items: slots }))
  vi.mocked(createTimeSlot).mockImplementation(async (input: TimeSlotInput) => {
    const slot = { ...input, id: 100 + slots.length }
    slots.push(slot)
    return slot
  })
  vi.mocked(updateTimeSlot).mockImplementation(async (id, input) => ({
    ...input,
    id,
  }))
  vi.mocked(getAvailabilityDefaults).mockResolvedValue({ items: [] })
  vi.mocked(setAvailabilityDefaults).mockImplementation(async (items) => ({
    items,
  }))
  vi.mocked(getPresets).mockResolvedValue({ items: [], total: 0 } as never)
  vi.mocked(completeSetup).mockResolvedValue({
    completed: true,
    recipe_count: 0,
    recipe_target: 10,
  })
  return { settings, getProfile: () => profile }
}

function renderWizard() {
  // No TooltipProvider here: /setup renders without the app shell.
  return renderWithRouter(<SetupWizard />)
}

const user = () => userEvent.setup()

async function next(u: ReturnType<typeof userEvent.setup>, step: number) {
  await u.click(screen.getByTestId("setup-next"))
  await screen.findByText(`Step ${step} of 7`)
}

describe("SetupWizard", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("en")
  })

  it("renders every step with its defaults and saves each on Next", async () => {
    const server = serve()
    const u = user()
    renderWizard()

    // 1 Welcome: the browser language is preselected.
    await screen.findByText("Step 1 of 7")
    expect(screen.getByRole("radio", { name: "English" })).toBeChecked()
    expect(screen.queryByTestId("setup-skip")).toBeNull()
    await next(u, 2)
    expect(server.getProfile().locale).toBe("en")

    // 2 You & household
    expect(screen.getByRole("radio", { name: "Omnivore" })).toBeChecked()
    expect(screen.getByTestId("setup-household-portions")).toHaveTextContent(
      "= 1 portion per plate"
    )
    expect(screen.getByTestId("setup-skip")).toHaveTextContent(
      "Skip, use defaults"
    )
    await next(u, 3)

    // 3 Nutrition goal: maintain preselected.
    expect(screen.getByRole("radio", { name: /^Maintain/ })).toBeChecked()
    await next(u, 4)
    expect(server.getProfile()).toMatchObject({
      kcal_target: 2200,
      protein_pct: 30,
      fat_pct: 30,
      carbs_pct: 40,
    })

    // 4 Meals & week: the three default slots, all planned.
    expect(screen.getByLabelText("Name of meal 1")).toHaveValue("Breakfast")
    expect(screen.getByLabelText("Name of meal 2")).toHaveValue("Lunch")
    expect(screen.getByLabelText("Name of meal 3")).toHaveValue("Dinner")
    expect(screen.getByRole("grid")).toBeInTheDocument()
    await next(u, 5)
    expect(createTimeSlot).toHaveBeenCalledTimes(3)
    expect(vi.mocked(createTimeSlot).mock.calls.map((c) => c[0])).toEqual([
      {
        name_key: "slot.breakfast",
        icon: "Coffee",
        sort_order: 0,
        active: true,
      },
      { name_key: "slot.lunch", icon: "Sun", sort_order: 1, active: true },
      { name_key: "slot.dinner", icon: "Moon", sort_order: 2, active: true },
    ])
    const grid = vi.mocked(setAvailabilityDefaults).mock.calls[0]![0]
    expect(grid).toHaveLength(21)
    expect(grid.every((c) => c.mode === "plan")).toBe(true)

    // 5 Cooking style
    expect(
      screen.getByRole("group", { name: "Cooking sessions per day" })
    ).toHaveTextContent("1")
    expect(screen.getByLabelText("Batch cooking")).toBeChecked()
    expect(
      screen.getByRole("group", { name: "Leftovers keep for" })
    ).toHaveTextContent("3 days")
    expect(screen.getByLabelText("Freezing")).not.toBeChecked()
    await next(u, 6)
    // The defaults were kept, so nothing becomes an override.
    expect(setSetting).not.toHaveBeenCalled()

    // 6 Variety & shopping: Saturday preselected.
    expect(
      screen.getByRole("group", { name: "No repeats within" })
    ).toHaveTextContent("3 weeks")
    expect(screen.getByTestId("setup-shopping-day")).toHaveTextContent(
      "Saturday"
    )
    expect(
      screen.getByRole("group", { name: "New recipes per week" })
    ).toHaveTextContent("0")
    await next(u, 7)
    expect(server.settings.get("plan.shopping_day")).toBe("5")
    expect(setSetting).toHaveBeenCalledTimes(1)

    // 7 Assistant: no key yet, so the calm note. Skip would equal Next.
    expect(screen.queryByTestId("setup-skip")).toBeNull()
    expect(
      screen.getByText("You can add this later. Planning works without it.")
    ).toBeInTheDocument()
    expect(screen.getByLabelText("Daily suggestion")).toBeChecked()
    expect(screen.getByLabelText("Notes to your assistant")).toHaveValue("")
    await u.click(screen.getByTestId("setup-next"))

    await screen.findByTestId("setup-finish")
    expect(server.settings.has("discovery.daily_enabled")).toBe(false)
    expect(screen.getByRole("heading", { name: "You're set up" })).toBeVisible()
  })

  it("the household helper shows 2 adults + 1 child as 2.5 portions", async () => {
    serve()
    const u = user()
    renderWizard()
    await screen.findByText("Step 1 of 7")
    await next(u, 2)

    await u.click(screen.getByRole("button", { name: "Increase Adults" }))
    await u.click(screen.getByRole("button", { name: "Increase Children" }))
    expect(screen.getByTestId("setup-household-portions")).toHaveTextContent(
      "= 2.5 portions per plate"
    )

    await next(u, 3)
    expect(updateProfile).toHaveBeenLastCalledWith(
      expect.objectContaining({ household_portions: 2.5 })
    )
  })

  it("Skip, use defaults on step 2 writes the step 3–6 defaults and reaches Finish", async () => {
    // A fresh install: every setting is at its registry default.
    const server = serve()
    const u = user()
    renderWizard()
    await screen.findByText("Step 1 of 7")
    await next(u, 2)

    await u.click(screen.getByRole("radio", { name: "Vegetarian" }))
    await u.click(screen.getByTestId("setup-skip"))

    await screen.findByTestId("setup-finish")
    expect(server.getProfile()).toMatchObject({
      diet_type: "vegetarian",
      kcal_target: 2200,
      protein_pct: 30,
      fat_pct: 30,
      carbs_pct: 40,
    })
    // The default slots are created; the usual week is left as stored.
    expect(createTimeSlot).toHaveBeenCalledTimes(3)
    expect(setAvailabilityDefaults).not.toHaveBeenCalled()
    // Only the suggested Saturday differs from the registry defaults.
    expect(vi.mocked(setSetting).mock.calls).toEqual([
      ["plan.shopping_day", "5"],
    ])
    expect(completeSetup).not.toHaveBeenCalled()
  })

  it("Add recipes completes setup, then opens Recipes", async () => {
    serve()
    const u = user()
    renderWizard()
    await screen.findByText("Step 1 of 7")
    await next(u, 2)
    await u.click(screen.getByTestId("setup-skip"))
    await screen.findByTestId("setup-finish")

    await u.click(screen.getByRole("button", { name: "Add recipes" }))
    await waitFor(() =>
      expect(navigate).toHaveBeenCalledWith({
        to: "/recipes",
        state: { focusMain: true },
      })
    )
    expect(completeSetup).toHaveBeenCalledTimes(1)
    expect(vi.mocked(completeSetup).mock.invocationCallOrder[0]).toBeLessThan(
      navigate.mock.invocationCallOrder[0]!
    )
  })

  it("stays on the step when saving fails", async () => {
    serve()
    vi.mocked(updateProfile).mockRejectedValueOnce(
      new ApiError(500, "error.server")
    )
    const u = user()
    renderWizard()
    await screen.findByText("Step 1 of 7")
    await u.click(screen.getByTestId("setup-next"))

    await waitFor(() => expect(updateProfile).toHaveBeenCalled())
    await waitFor(() => expect(screen.getByTestId("setup-next")).toBeEnabled())
    expect(screen.getByText("Step 1 of 7")).toBeInTheDocument()
  })

  it("Skip on a re-run keeps the stored nutrition goal", async () => {
    const server = serve(
      {},
      { kcal_target: 1800, protein_pct: 35, fat_pct: 30, carbs_pct: 35 }
    )
    const u = user()
    renderWizard()
    await screen.findByText("Step 1 of 7")
    await next(u, 2)
    await u.click(screen.getByTestId("setup-skip"))

    await screen.findByTestId("setup-finish")
    expect(server.getProfile()).toMatchObject({
      kcal_target: 1800,
      protein_pct: 35,
      fat_pct: 30,
      carbs_pct: 35,
    })
    // usePatchProfile sends the full profile; the goal must be the stored one.
    for (const [input] of vi.mocked(updateProfile).mock.calls) {
      expect(input.kcal_target).toBe(1800)
    }
  })

  it("Skip on a re-run keeps the stored cooking, variety and daily settings", async () => {
    serve()
    vi.mocked(listSettings).mockResolvedValue(
      mockSettingsList([
        { key: "plan.default_keep_days", value: "5", source: "db" },
        { key: "plan.variety_weeks", value: "2", source: "db" },
        { key: "plan.shopping_day", value: "3", source: "db" },
        { key: "discovery.daily_enabled", value: "false", source: "db" },
      ])
    )
    const u = user()
    renderWizard()
    await screen.findByText("Step 1 of 7")
    await next(u, 2)
    await u.click(screen.getByTestId("setup-skip"))

    await screen.findByTestId("setup-finish")
    expect(setSetting).not.toHaveBeenCalled()
  })

  it("Skip on a re-run keeps the stored slots and usual week", async () => {
    serve()
    vi.mocked(listTimeSlots).mockResolvedValue({
      items: [
        {
          id: 7,
          name_key: "slot.dinner",
          icon: "Moon",
          sort_order: 0,
          active: true,
        },
      ],
    })
    vi.mocked(getAvailabilityDefaults).mockResolvedValue({
      items: [0, 1, 2, 3, 4, 5, 6].map((weekday) => ({
        weekday,
        slot_id: 7,
        mode: weekday < 5 ? ("routine" as const) : ("plan" as const),
        preset_id: weekday < 5 ? 3 : null,
      })),
    })
    const u = user()
    renderWizard()
    await screen.findByText("Step 1 of 7")
    await next(u, 2)
    await u.click(screen.getByTestId("setup-skip"))

    await screen.findByTestId("setup-finish")
    expect(createTimeSlot).not.toHaveBeenCalled()
    expect(updateTimeSlot).not.toHaveBeenCalled()
    expect(setAvailabilityDefaults).not.toHaveBeenCalled()
  })

  it("re-run opens prefilled with the stored slots and values", async () => {
    serve()
    vi.mocked(getProfile).mockResolvedValue({
      ...mockProfile,
      diet_type: "vegan",
      kcal_target: 1800,
      protein_pct: 35,
      fat_pct: 30,
      carbs_pct: 35,
    })
    vi.mocked(listTimeSlots).mockResolvedValue({
      items: [
        {
          id: 7,
          name_key: "slot.dinner",
          icon: "Moon",
          sort_order: 0,
          active: true,
        },
      ],
    })
    const u = user()
    renderWizard()
    await screen.findByText("Step 1 of 7")
    await next(u, 2)
    expect(screen.getByRole("radio", { name: "Vegan" })).toBeChecked()
    await next(u, 3)
    expect(screen.getByRole("radio", { name: /^Cut/ })).toBeChecked()
    await next(u, 4)
    const list = screen.getByTestId("setup-slot-0").closest("ul")!
    expect(within(list).getAllByRole("listitem")).toHaveLength(1)
    expect(screen.getByLabelText("Name of meal 1")).toHaveValue("Dinner")
    await next(u, 5)
    expect(createTimeSlot).not.toHaveBeenCalled()
    expect(updateTimeSlot).toHaveBeenCalledWith(7, {
      name_key: "slot.dinner",
      icon: "Moon",
      sort_order: 0,
      active: true,
    })
  })

  it("re-run lists inactive slots and keeps sort_order over all slots", async () => {
    serve()
    vi.mocked(listTimeSlots).mockResolvedValue({
      items: [
        {
          id: 3,
          name_key: "slot.dinner",
          icon: "Moon",
          sort_order: 2,
          active: true,
        },
        {
          id: 2,
          name_key: "slot.lunch",
          icon: "Sun",
          sort_order: 1,
          active: false,
        },
        {
          id: 1,
          name_key: "slot.breakfast",
          icon: "Coffee",
          sort_order: 0,
          active: true,
        },
      ],
    })
    const u = user()
    renderWizard()
    await screen.findByText("Step 1 of 7")
    await next(u, 2)
    await next(u, 3)
    await next(u, 4)

    expect(screen.getByLabelText("Name of meal 1")).toHaveValue("Breakfast")
    expect(screen.getByLabelText("Name of meal 2")).toHaveValue("Lunch")
    expect(screen.getByLabelText("Name of meal 3")).toHaveValue("Dinner")
    expect(screen.getByRole("switch", { name: "Plan Lunch" })).not.toBeChecked()
    // The usual week covers only the slots that are on.
    const grid = screen.getByRole("grid", { name: "Your usual week" })
    expect(
      within(grid)
        .getAllByRole("rowheader")
        .map((h) => h.textContent)
    ).toEqual(["Breakfast", "Dinner"])

    await next(u, 5)
    expect(createTimeSlot).not.toHaveBeenCalled()
    expect(vi.mocked(updateTimeSlot).mock.calls).toEqual([
      [
        1,
        {
          name_key: "slot.breakfast",
          icon: "Coffee",
          sort_order: 0,
          active: true,
        },
      ],
      [
        2,
        { name_key: "slot.lunch", icon: "Sun", sort_order: 1, active: false },
      ],
      [
        3,
        { name_key: "slot.dinner", icon: "Moon", sort_order: 2, active: true },
      ],
    ])
  })

  it("switching to German in step 1 shows German meal names and keeps the slot keys", async () => {
    serve()
    const u = user()
    renderWizard()
    await screen.findByText("Step 1 of 7")
    await u.click(screen.getByRole("radio", { name: "Deutsch" }))
    await u.click(screen.getByTestId("setup-next"))
    await screen.findByText("Schritt 2 von 7")
    for (const step of [3, 4]) {
      await u.click(screen.getByTestId("setup-next"))
      await screen.findByText(`Schritt ${step} von 7`)
    }

    expect(screen.getByLabelText("Name der Mahlzeit 1")).toHaveValue(
      "Frühstück"
    )
    expect(screen.getByLabelText("Name der Mahlzeit 2")).toHaveValue(
      "Mittagessen"
    )
    expect(screen.getByLabelText("Name der Mahlzeit 3")).toHaveValue(
      "Abendessen"
    )
    await u.click(screen.getByTestId("setup-next"))
    await screen.findByText("Schritt 5 von 7")
    expect(
      vi.mocked(createTimeSlot).mock.calls.map((c) => c[0].name_key)
    ).toEqual(["slot.breakfast", "slot.lunch", "slot.dinner"])
  })

  it("a re-run in German keeps stored slot keys and stores a real rename", async () => {
    serve()
    await i18n.changeLanguage("de")
    vi.mocked(listTimeSlots).mockResolvedValue({
      items: [
        {
          id: 1,
          name_key: "slot.breakfast",
          icon: "Coffee",
          sort_order: 0,
          active: true,
        },
        {
          id: 2,
          name_key: "slot.dinner",
          icon: "Moon",
          sort_order: 1,
          active: true,
        },
      ],
    })
    const u = user()
    renderWizard()
    await screen.findByText("Schritt 1 von 7")
    for (const step of [2, 3, 4]) {
      await u.click(screen.getByTestId("setup-next"))
      await screen.findByText(`Schritt ${step} von 7`)
    }
    expect(screen.getByLabelText("Name der Mahlzeit 1")).toHaveValue(
      "Frühstück"
    )
    const dinner = screen.getByLabelText("Name der Mahlzeit 2")
    await u.clear(dinner)
    await u.type(dinner, "Brotzeit")
    await u.click(screen.getByTestId("setup-next"))
    await screen.findByText("Schritt 5 von 7")
    expect(
      vi
        .mocked(updateTimeSlot)
        .mock.calls.map(([id, input]) => [id, input.name_key])
    ).toEqual([
      [1, "slot.breakfast"],
      [2, "Brotzeit"],
    ])
  })

  it("the assistant step renders with a provider set from the environment", async () => {
    serve()
    vi.mocked(listSettings).mockResolvedValue(
      mockSettingsList([
        { key: "ai.provider", value: "fake", source: "env" },
        { key: "ai.api_key", source: "env", is_secret: true },
      ])
    )
    const u = user()
    renderWizard()
    await screen.findByText("Step 1 of 7")
    for (let step = 2; step <= 7; step++) await next(u, step)

    expect(screen.getByTestId("ai-wizard-step-1")).toBeInTheDocument()
    expect(screen.queryByTestId("setup-assistant-later")).toBeNull()
  })
})
