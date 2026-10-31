import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { Today } from "@/lib/api/today"
import i18n from "@/lib/i18n"
import { mockPlan, mockTodayByVerdict } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/today", () => ({ getToday: vi.fn() }))
vi.mock("@/lib/api/plan", () => ({
  getPlan: vi.fn(),
  clearPlan: vi.fn(),
  restorePlan: vi.fn(),
}))
vi.mock("@/lib/api/feedback", () => ({
  putFeedback: vi.fn(),
  deleteFeedback: vi.fn(),
}))
vi.mock("@/lib/api/foods", () => ({
  // Chili: 520 kcal per portion.
  listFoodMacros: vi.fn().mockResolvedValue({
    foods: [
      {
        food_id: 42,
        macros: { kcal: 520, protein: 28, fat: 18, carbs: 60 },
      },
    ],
  }),
}))
const toastMock = vi.hoisted(() =>
  Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn() })
)
vi.mock("@/lib/toast", () => ({ toast: toastMock, toastError: vi.fn() }))

import { deleteFeedback, putFeedback } from "@/lib/api/feedback"
import { getPlan } from "@/lib/api/plan"
import { getToday } from "@/lib/api/today"

import { TodayPage } from "./TodayPage"

const MONDAY = "2026-11-02"
const SUNDAY = "2026-11-01"

function renderToday(today: Today, real = MONDAY) {
  vi.mocked(getToday).mockResolvedValue(today)
  return renderWithRouter(<TodayPage date={today.date} today={real} />)
}

describe("TodayPage", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    vi.mocked(getPlan).mockResolvedValue(mockPlan)
    await i18n.changeLanguage("en")
  })

  it("renders the verdict from /api/today while the plan still loads", async () => {
    vi.mocked(getPlan).mockReturnValue(new Promise(() => {}))
    renderToday(mockTodayByVerdict.nothing_to_cook)

    expect(await screen.findByTestId("today-verdict")).toHaveTextContent(
      "Nothing to cook today"
    )
    expect(screen.getByTestId("today-rate")).toBeInTheDocument()
    expect(screen.getByTestId("today-plan-skeleton")).toBeInTheDocument()
    expect(screen.queryByTestId("today-leftover-card")).toBeNull()
  })

  it("keeps a plan error inside the plan region", async () => {
    vi.mocked(getPlan).mockRejectedValue(new Error("offline"))
    renderToday(mockTodayByVerdict.nothing_to_cook)

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Couldn't load this day"
    )
    expect(screen.getByTestId("today-verdict")).toHaveTextContent(
      "Nothing to cook today"
    )
    expect(screen.getByTestId("today-rate")).toBeInTheDocument()
  })

  it("asks no rating question on a future day", async () => {
    renderToday(mockTodayByVerdict.nothing_to_cook, SUNDAY)
    await screen.findByTestId("today-leftover-card")
    expect(screen.queryByTestId("today-rate")).toBeNull()
  })

  // F5-AC1 (AC-1 step 4).
  it("Monday: nothing to cook, the leftover card and Dinner 3/3", async () => {
    renderToday(mockTodayByVerdict.nothing_to_cook)

    expect(await screen.findByTestId("today-verdict")).toHaveTextContent(
      "Nothing to cook today"
    )
    expect(screen.getByTestId("today-date")).toHaveTextContent(
      "Monday, November 2"
    )
    expect(getPlan).toHaveBeenCalledWith("2026-10-31")

    const card = await screen.findByTestId("today-leftover-card")
    expect(
      within(card).getByRole("heading", { name: "Chili, leftover" })
    ).toBeInTheDocument()
    expect(card.querySelector("[data-batch-badge]")).toHaveTextContent(
      "2/3 · use by Wed"
    )
    expect(
      await within(card).findByText("Reheat 2 portions · 520 kcal each")
    ).toBeInTheDocument()

    const dinner = screen.getByTestId("today-meal-3")
    expect(dinner).toHaveTextContent("Dinner")
    expect(dinner.querySelector("[data-batch-badge]")).toHaveTextContent("3/3")
    expect(screen.queryByTestId("today-meal-2")).toBeNull()
    expect(screen.getByTestId("today-meal-1")).toHaveTextContent(
      "Nothing planned"
    )
  })

  // F5-AC1 (AC-1 step 5).
  it("Sunday: Cook Chili × 6, and Start cooking opens cook mode", async () => {
    renderToday(mockTodayByVerdict.cook, SUNDAY)

    expect(await screen.findByTestId("today-verdict")).toHaveTextContent(
      "Cook Chili × 6"
    )
    const card = await screen.findByTestId("today-cook-card")
    expect(card.querySelector("[data-batch-badge]")).toHaveTextContent(
      "Cook × 6"
    )
    expect(card).toHaveTextContent("Covers Sun dinner, Mon lunch, Mon dinner")
    expect(card).toHaveTextContent("About 60 min")
    expect(
      within(card).getByRole("link", { name: "Start cooking" })
    ).toHaveAttribute("href", "/cook/7")
    expect(screen.queryByTestId("today-leftover-card")).toBeNull()
  })

  // F6-AC4: once Done cooking marked the batch, "Cooked" replaces Start.
  it("a cooked batch says Cooked instead of Start cooking", async () => {
    const base = mockTodayByVerdict.cook
    renderToday(
      {
        ...base,
        cook: base.cook.map((c) => ({
          ...c,
          session: { ...c.session, cooked_at: "2026-11-01T18:30:00Z" },
        })),
      },
      SUNDAY
    )

    const card = await screen.findByTestId("today-cook-card")
    expect(within(card).getByTestId("today-cooked")).toHaveTextContent("Cooked")
    expect(
      within(card).queryByRole("link", { name: "Start cooking" })
    ).toBeNull()
  })

  // F5-AC3: the answer writes feedback, the question goes, Undo clears it.
  it("rates the last meal, hides the question and offers Undo", async () => {
    const user = userEvent.setup()
    vi.mocked(putFeedback).mockResolvedValue({
      plate_id: 1,
      status: "loved",
      rated_at: "2026-11-02T10:00:00Z",
    })
    vi.mocked(deleteFeedback).mockResolvedValue(undefined)
    renderToday(mockTodayByVerdict.nothing_to_cook)

    const question = await screen.findByTestId("today-rate")
    expect(question).toHaveTextContent("How was Sunday's Chili?")
    expect(
      within(question)
        .getAllByRole("button")
        .map((b) => b.textContent)
    ).toEqual(["Loved it", "Not again", "Didn't eat it"])

    await user.click(within(question).getByRole("button", { name: "Loved it" }))
    expect(putFeedback).toHaveBeenCalledWith(1, { status: "loved" })
    expect(screen.queryByTestId("today-rate")).toBeNull()

    await waitFor(() => expect(toastMock).toHaveBeenCalled())
    const [message, options] = toastMock.mock.calls[0]
    expect(message).toBe("Rating saved")
    options.action.onClick()
    await waitFor(() => expect(deleteFeedback).toHaveBeenCalledWith(1))
  })

  // F5-AC4.
  it("shows the defrost reminder when tomorrow eats frozen portions", async () => {
    renderToday({
      ...mockTodayByVerdict.nothing_to_cook,
      reminders: [
        {
          kind: "defrost",
          session_id: 7,
          food_name: "Chili",
          date: "2026-11-03",
          slot_id: 3,
        },
      ],
    })
    const reminder = await screen.findByTestId("today-reminder-defrost")
    expect(reminder).toHaveTextContent("Freezer")
    expect(reminder).toHaveTextContent("Take Chili out of the freezer tonight")
  })

  it("hides reminders, the banner and the question when there are none", async () => {
    renderToday({ ...mockTodayByVerdict.no_plates, rate: null })
    await screen.findByTestId("today-verdict")
    expect(screen.queryByRole("heading", { name: "Reminders" })).toBeNull()
    expect(screen.queryByTestId("today-plan-banner")).toBeNull()
    expect(screen.queryByTestId("today-rate")).toBeNull()
  })

  it("no plan: Plan this week and the Plan next week banner", async () => {
    renderToday(mockTodayByVerdict.no_plan)
    expect(await screen.findByTestId("today-verdict")).toHaveTextContent(
      "No plan for today"
    )
    expect(
      within(screen.getByTestId("today-no-plan")).getByRole("link", {
        name: "Plan this week",
      })
    ).toHaveAttribute("href", "/plan?start=2026-11-07")
    expect(
      within(screen.getByTestId("today-plan-banner")).getByRole("link", {
        name: "Plan next week",
      })
    ).toHaveAttribute("href", "/plan?start=2026-11-14")
  })

  it("the banner says when the next week starts today or tomorrow", async () => {
    const banner = async () =>
      (await screen.findByTestId("today-plan-banner")).textContent

    const a = renderToday(mockTodayByVerdict.no_plan, "2026-11-12")
    expect(await banner()).toContain(
      "Your next week starts Saturday, November 14."
    )
    a.unmount()

    const b = renderToday(mockTodayByVerdict.no_plan, "2026-11-13")
    expect(await banner()).toContain("Your next week starts tomorrow.")
    b.unmount()

    await i18n.changeLanguage("de")
    renderToday(mockTodayByVerdict.no_plan, "2026-11-14")
    expect(await banner()).toContain("Deine nächste Woche beginnt heute.")
  })

  it("below the recipe target: Add N more recipes", async () => {
    renderToday({
      ...mockTodayByVerdict.no_plan,
      setup: { recipe_count: 7, recipe_target: 10 },
    })
    const card = await screen.findByTestId("today-setup-card")
    expect(card).toHaveTextContent("Add 3 more recipes to start planning")
    expect(
      within(card).getByRole("link", { name: "Add recipes" })
    ).toHaveAttribute("href", "/recipes")
    expect(screen.queryByTestId("today-no-plan")).toBeNull()
  })

  // F5-AC6: the Settings gear lives in the Today header on phones.
  it("carries the phone Settings gear and day navigation", async () => {
    renderToday(mockTodayByVerdict.nothing_to_cook, SUNDAY)
    const gear = await screen.findByTestId("today-settings")
    expect(gear).toHaveAttribute("href", "/settings")
    expect(gear).toHaveClass("md:hidden")
    expect(screen.getByRole("link", { name: "Previous day" })).toHaveAttribute(
      "href",
      "/today?date=2026-11-01"
    )
    expect(screen.getByRole("link", { name: "Next day" })).toHaveAttribute(
      "href",
      "/today?date=2026-11-03"
    )
    expect(screen.getByRole("link", { name: "Back to today" })).toHaveAttribute(
      "href",
      "/today"
    )
  })

  it("the week strip links each day to the plan at that day", async () => {
    renderToday(mockTodayByVerdict.nothing_to_cook)
    const strip = await screen.findByRole("navigation", { name: "This week" })
    const days = within(strip).getAllByRole("link")
    expect(days).toHaveLength(7)
    expect(days[2]).toHaveAttribute(
      "href",
      "/plan?start=2026-10-31&day=2026-11-02"
    )
    expect(days[2]).toHaveAttribute("aria-current", "date")
    expect(days[1].querySelector("[data-batch-badge]")).toHaveTextContent(
      "Cook × 6"
    )
  })

  it("renders the verdict in German", async () => {
    await i18n.changeLanguage("de")
    renderToday(mockTodayByVerdict.cook, SUNDAY)
    expect(await screen.findByTestId("today-verdict")).toHaveTextContent(
      "Heute kochen: Chili × 6"
    )
    expect(
      await screen.findByRole("link", { name: "Kochen starten" })
    ).toBeInTheDocument()
  })
})
