import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { Plan } from "@/lib/api/plan"
import i18n from "@/lib/i18n"
import {
  mockPlan,
  mockRemovedBatch,
  mockSessionWithWarnings,
} from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/plan", () => ({
  getPlan: vi.fn(),
  clearPlan: vi.fn(),
  restorePlan: vi.fn(),
}))
vi.mock("@/lib/api/availability", () => ({
  setAvailabilityOverride: vi.fn(),
  clearAvailabilityOverride: vi.fn(),
}))
vi.mock("@/lib/api/setup", () => ({
  getSetupStatus: vi.fn().mockResolvedValue({
    completed: true,
    recipe_count: 12,
    recipe_target: 10,
  }),
}))
vi.mock("@/lib/api/settings", () => ({
  listSettings: vi
    .fn()
    .mockResolvedValue({ items: [], cipher_available: true }),
}))
vi.mock("@/lib/api/foods", () => ({
  listFoods: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  listFoodMacros: vi.fn().mockResolvedValue({ foods: [] }),
  getFood: vi.fn(),
}))
vi.mock("@/lib/api/presets", () => ({
  getPresets: vi
    .fn()
    .mockResolvedValue({ items: [], total: 0, known_tags: [] }),
}))
const navigateMock = vi.hoisted(() => vi.fn())
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useNavigate: () => navigateMock,
}))
// The chat panel has its own tests; here only its opening matters.
vi.mock("@/components/chat/ChatPanel", () => ({ ChatPanel: () => null }))

const toastMock = vi.hoisted(() =>
  Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn() })
)
vi.mock("@/lib/toast", () => ({ toast: toastMock, toastError: vi.fn() }))

import {
  clearAvailabilityOverride,
  setAvailabilityOverride,
} from "@/lib/api/availability"
import { clearPlan, getPlan, restorePlan } from "@/lib/api/plan"
import { useChatUI } from "@/lib/stores/chat-ui"

import { PlanPage } from "./PlanPage"

interface ToastOptions {
  action: { label: string; onClick: () => void }
}

function lastToast() {
  const call = toastMock.mock.calls.at(-1)!
  return { message: call[0] as string, opts: call[1] as ToastOptions }
}

const emptyPlan: Plan = {
  ...mockPlan,
  cells: mockPlan.cells.map((c) => ({ ...c, plate: null })),
  sessions: [],
  stats: { plates: 0, cook_days: 0, warnings: 0 },
}

beforeEach(async () => {
  vi.clearAllMocks()
  await i18n.changeLanguage("en")
  vi.mocked(getPlan).mockResolvedValue(mockPlan)
  vi.mocked(restorePlan).mockResolvedValue({ plate_ids: [1, 2, 3] })
  useChatUI.setState({ open: false })
})

describe("PlanPage", () => {
  it("shows a skeleton grid while the plan loads", async () => {
    vi.mocked(getPlan).mockReturnValue(new Promise(() => {}))
    renderWithRouter(<PlanPage />)
    expect(
      await screen.findByRole("status", { name: "Loading the plan" })
    ).toBeInTheDocument()
  })

  it("opens the anchored window without start and the given one with it", async () => {
    renderWithRouter(<PlanPage />)
    await screen.findByRole("grid")
    expect(getPlan).toHaveBeenCalledWith(undefined)

    renderWithRouter(<PlanPage start="2026-11-07" />)
    await waitFor(() => expect(getPlan).toHaveBeenCalledWith("2026-11-07"))
  })

  it("renders the toolbar, the cook strip and the AC-1 badges", async () => {
    renderWithRouter(<PlanPage />)
    const grid = await screen.findByRole("grid")
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(/31.*6/)
    expect(screen.getByText("3 plates, 1 cook day")).toBeInTheDocument()
    expect(screen.getByRole("region", { name: "Cook days" })).toHaveTextContent(
      /Chili × 6/
    )
    expect(within(grid).getByText("Cook × 6")).toBeInTheDocument()
    expect(within(grid).getByText("2/3")).toBeInTheDocument()
    expect(within(grid).getByText("3/3")).toBeInTheDocument()
  })

  it("links the week arrows, This week and Shop to their windows", async () => {
    renderWithRouter(<PlanPage />)
    await screen.findByRole("grid")
    expect(screen.getByRole("link", { name: "Previous week" })).toHaveAttribute(
      "href",
      "/plan?start=2026-10-24"
    )
    expect(screen.getByRole("link", { name: "Next week" })).toHaveAttribute(
      "href",
      "/plan?start=2026-11-07"
    )
    const thisWeek = screen.getByRole("link", { name: "This week" })
    expect(thisWeek).toHaveAttribute("href", "/plan")
    expect(thisWeek).toHaveAttribute("aria-current", "page")
    expect(screen.getByRole("link", { name: "Shop" })).toHaveAttribute(
      "href",
      "/shop?from=2026-10-31&to=2026-11-06"
    )
  })

  it("F3-AC6: Clear this week removes the window and Undo restores it with its batches", async () => {
    const user = userEvent.setup()
    vi.mocked(clearPlan).mockResolvedValue(mockRemovedBatch)
    renderWithRouter(<PlanPage />)
    await screen.findByRole("grid")

    await user.click(screen.getByRole("button", { name: "More plan actions" }))
    await user.click(
      await screen.findByRole("menuitem", { name: "Clear this week" })
    )
    await waitFor(() =>
      expect(clearPlan).toHaveBeenCalledWith("2026-10-31", "2026-11-06")
    )
    await waitFor(() => expect(toastMock).toHaveBeenCalled())
    const { message, opts } = lastToast()
    expect(message).toBe("Plan cleared")

    opts.action.onClick()
    await waitFor(() =>
      expect(restorePlan).toHaveBeenCalledWith(mockRemovedBatch.removed)
    )
    expect(mockRemovedBatch.removed.sessions).toHaveLength(1)
  })

  it("N marks the focused cell No plate, and Undo brings the plate back", async () => {
    const user = userEvent.setup()
    vi.mocked(setAvailabilityOverride).mockResolvedValue({
      cell: { ...mockPlan.cells[0], mode: "none" },
      removed: mockRemovedBatch.removed,
    })
    vi.mocked(clearAvailabilityOverride).mockResolvedValue(undefined)
    renderWithRouter(<PlanPage />)
    ;(
      await screen.findByRole("button", {
        name: "Monday lunch, Chili, leftover 2 of 3, confirmed",
      })
    ).focus()
    await user.keyboard("n")
    await waitFor(() =>
      expect(setAvailabilityOverride).toHaveBeenCalledWith("2026-11-02", 2, {
        mode: "none",
        note: null,
        remove_plate: true,
      })
    )
    await waitFor(() => expect(toastMock).toHaveBeenCalled())
    expect(lastToast().message).toBe("Marked as No plate")
    lastToast().opts.action.onClick()
    await waitFor(() =>
      expect(restorePlan).toHaveBeenCalledWith(mockRemovedBatch.removed)
    )
    expect(clearAvailabilityOverride).toHaveBeenCalledWith("2026-11-02", 2)
  })

  it("teaches an empty window; Plan this week opens the wizard, or plan by hand focuses the first open cell", async () => {
    const user = userEvent.setup()
    vi.mocked(getPlan).mockResolvedValue(emptyPlan)
    renderWithRouter(<PlanPage />)
    expect(
      await screen.findByRole("link", { name: "Plan this week" })
    ).toHaveAttribute("href", `/plan/new?start=${emptyPlan.window.start}`)
    await user.click(
      await screen.findByRole("button", { name: "or plan by hand" })
    )
    expect(document.activeElement).toHaveAccessibleName(
      "Saturday breakfast, nothing planned"
    )
    expect(
      screen.getByRole("button", { name: "Copy a previous week" })
    ).toBeInTheDocument()
  })

  it("Ask assistant opens the chat", async () => {
    const user = userEvent.setup()
    renderWithRouter(<PlanPage />)
    await user.click(
      await screen.findByRole("button", { name: "Ask assistant" })
    )
    expect(useChatUI.getState().open).toBe(true)
  })

  it("the warning count steps through the cells that need attention", async () => {
    const user = userEvent.setup()
    vi.mocked(getPlan).mockResolvedValue({
      ...mockPlan,
      sessions: [{ ...mockSessionWithWarnings, id: 7 }],
    })
    renderWithRouter(<PlanPage />)
    const count = await screen.findByRole("button", {
      name: "2 plates need attention",
    })
    // One press focuses the next cell, the second opens its sheet (F2).
    await user.click(count)
    expect(document.activeElement).toHaveAccessibleName(/^Sunday dinner, Chili/)
    await user.click(count)
    expect(await screen.findByTestId("plan-sheet")).toHaveTextContent(
      "Sunday dinner"
    )
    await user.keyboard("{Escape}")
    await waitFor(() =>
      expect(screen.queryByTestId("plan-sheet")).not.toBeInTheDocument()
    )
    await user.click(count)
    expect(document.activeElement).toHaveAccessibleName(/^Monday lunch, Chili/)
    await user.click(count)
    expect(await screen.findByTestId("plan-sheet")).toHaveTextContent(
      "Monday lunch"
    )
    await user.keyboard("{Escape}")
    await waitFor(() =>
      expect(screen.queryByTestId("plan-sheet")).not.toBeInTheDocument()
    )
    await user.click(count)
    expect(document.activeElement).toHaveAccessibleName(/^Sunday dinner, Chili/)
  })

  it("keeps 44 px toolbar targets except for a fine pointer from 768 px", async () => {
    vi.mocked(getPlan).mockResolvedValue({
      ...mockPlan,
      sessions: [{ ...mockSessionWithWarnings, id: 7 }],
    })
    renderWithRouter(<PlanPage />)
    await screen.findByRole("grid")
    const controls = [
      screen.getByRole("button", { name: "Ask assistant" }),
      screen.getByRole("link", { name: "Shop" }),
      screen.getByRole("button", { name: "More plan actions" }),
      screen.getByRole("link", { name: "Previous week" }),
      screen.getByRole("link", { name: "This week" }),
      screen.getByRole("link", { name: "Next week" }),
      screen.getByTestId("plan-warnings"),
    ]
    for (const el of controls) {
      const classes = el.className.split(/\s+/)
      expect(classes.some((c) => /^(h|min-h|size)-11$/.test(c))).toBe(true)
      // Any smaller size applies only to a fine pointer.
      for (const c of classes.filter((c) => /^md:/.test(c))) {
        expect(c).toMatch(/^md:pointer-fine:/)
      }
    }
  })

  it("? opens the keyboard shortcuts", async () => {
    const user = userEvent.setup()
    renderWithRouter(<PlanPage />)
    await screen.findByRole("grid")
    await user.keyboard("?")
    expect(
      await screen.findByRole("dialog", { name: "Keyboard shortcuts" })
    ).toHaveTextContent("Mark as No plate")
  })

  it("keeps a past window read-only", async () => {
    vi.mocked(getPlan).mockResolvedValue({
      ...emptyPlan,
      window: { ...mockPlan.window, today: "2026-12-01", is_current: false },
    })
    renderWithRouter(<PlanPage start="2026-10-31" />)
    await screen.findByRole("grid")
    expect(
      screen.getByText("This week is over. You can still rate its meals.")
    ).toBeInTheDocument()
    expect(screen.queryByTestId("plan-empty")).not.toBeInTheDocument()
    expect(screen.queryByText("Add")).not.toBeInTheDocument()
  })

  it("a plate cell opens its plate sheet, and Open batch moves to the cook cell", async () => {
    const user = userEvent.setup()
    renderWithRouter(<PlanPage />)
    await user.click(
      await screen.findByRole("button", {
        name: "Monday dinner, Chili, leftover 3 of 3, confirmed",
      })
    )
    const sheet = await screen.findByRole("dialog", { name: "Chili" })
    expect(sheet).toHaveTextContent("Monday dinner")
    expect(sheet).toHaveTextContent("From Sun · Chili (3 of 3)")
    await user.click(within(sheet).getByRole("button", { name: "Open batch" }))
    await waitFor(() =>
      expect(screen.getByRole("dialog", { name: "Chili" })).toHaveTextContent(
        "Sunday dinner"
      )
    )
    expect(
      screen.getByRole("button", { name: "Increase Portions to cook" })
    ).toBeInTheDocument()
  })

  it("an empty cell opens its sheet, and Add food switches to the picker in place", async () => {
    const user = userEvent.setup()
    renderWithRouter(<PlanPage />)
    await user.click(
      await screen.findByRole("button", {
        name: "Tuesday lunch, nothing planned",
      })
    )
    const sheet = await screen.findByRole("dialog", { name: "Tuesday lunch" })
    await user.click(within(sheet).getByRole("button", { name: "Add food" }))
    expect(
      await screen.findByRole("dialog", { name: "Add food" })
    ).toContainElement(screen.getByTestId("food-picker"))
    expect(screen.getAllByRole("dialog")).toHaveLength(1)
    await user.click(screen.getByRole("button", { name: "Back" }))
    expect(
      await screen.findByRole("dialog", { name: "Tuesday lunch" })
    ).toBeInTheDocument()
  })

  it("a past window opens no sheet for an empty cell", async () => {
    const user = userEvent.setup()
    vi.mocked(getPlan).mockResolvedValue({
      ...mockPlan,
      window: { ...mockPlan.window, today: "2026-12-01", is_current: false },
    })
    renderWithRouter(<PlanPage start="2026-10-31" />)
    await user.click(
      await screen.findByRole("button", {
        name: "Tuesday lunch, nothing planned",
      })
    )
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument()
  })

  it("returns focus to the cell when the sheet closes", async () => {
    const user = userEvent.setup()
    renderWithRouter(<PlanPage />)
    const cell = await screen.findByRole("button", {
      name: "Monday dinner, Chili, leftover 3 of 3, confirmed",
    })
    cell.focus()
    await user.keyboard("{Enter}")
    expect(await screen.findByRole("dialog", { name: "Chili" })).toBeVisible()
    await user.keyboard("{Escape}")
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument()
    )
    expect(cell).toHaveFocus()
  })

  it("has a 44 px, translated close button", async () => {
    const user = userEvent.setup()
    await i18n.changeLanguage("de")
    renderWithRouter(<PlanPage />)
    await user.click(
      await screen.findByRole("button", { name: /Chili, Rest 3 von 3/ })
    )
    const close = await screen.findByTestId("plan-sheet-close")
    expect(close).toHaveAccessibleName("Schließen")
    expect(close.className.split(/\s+/)).toContain("size-11")
    expect(screen.queryAllByRole("button", { name: "Close" })).toHaveLength(0)
  })

  it("opens the cell given in ?cell=", async () => {
    renderWithRouter(<PlanPage cell="2026-11-02|2" />)
    expect(
      await screen.findByRole("dialog", { name: "Chili" })
    ).toHaveTextContent("Monday lunch")
  })

  it("focuses the first cell of the day given in ?day= (Today's week strip)", async () => {
    renderWithRouter(<PlanPage start="2026-10-31" day="2026-11-02" />)
    await screen.findByRole("grid")
    await waitFor(() =>
      expect(document.activeElement).toHaveAccessibleName(
        "Monday breakfast, nothing planned"
      )
    )
  })

  it("carries the compact Generate button on phones only (D31)", async () => {
    renderWithRouter(<PlanPage />)
    await screen.findByRole("grid")
    const generate = screen.getByTestId("generate-plan-compact")
    expect(generate.parentElement).toHaveClass("md:hidden")
  })

  it("Open batch to a cook cell before the window opens it in the previous window", async () => {
    const user = userEvent.setup()
    // The window starts on Monday; the batch cooked on Sunday is in the
    // previous window.
    const days = [
      "2026-11-02",
      "2026-11-03",
      "2026-11-04",
      "2026-11-05",
      "2026-11-06",
      "2026-11-07",
      "2026-11-08",
    ]
    vi.mocked(getPlan).mockResolvedValue({
      ...mockPlan,
      window: {
        ...mockPlan.window,
        start: "2026-11-02",
        end: "2026-11-08",
        prev_start: "2026-10-26",
        next_start: "2026-11-09",
      },
      cells: days.flatMap((date) =>
        [1, 2, 3].map(
          (slot_id) =>
            mockPlan.cells.find(
              (c) => c.date === date && c.slot_id === slot_id
            ) ?? { ...mockPlan.cells[0], date, slot_id, plate: null }
        )
      ),
      days: mockPlan.days.filter((d) => days.includes(d.date)),
    })
    renderWithRouter(<PlanPage start="2026-11-02" />)
    await user.click(
      await screen.findByRole("button", {
        name: "Monday lunch, Chili, leftover 2 of 3, confirmed",
      })
    )
    const sheet = await screen.findByRole("dialog", { name: "Chili" })
    await user.click(within(sheet).getByRole("button", { name: "Open batch" }))
    expect(navigateMock).toHaveBeenCalledWith({
      to: "/plan",
      search: { start: "2026-10-26", cell: "2026-11-01|3" },
    })
  })
})
