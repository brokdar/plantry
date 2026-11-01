import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { Plan } from "@/lib/api/plan"
import type { Plate } from "@/lib/api/plates"
import i18n from "@/lib/i18n"
import {
  mockDraftPlan,
  mockPlan,
  mockPlateComponent,
  mockRun,
} from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

// Phase 3 F2: the review board on top of the Phase 2 board (D33–D35).

vi.mock("@/lib/api/plan", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/plan")>()),
  getPlan: vi.fn(),
  restorePlan: vi.fn(),
  acceptPlan: vi.fn(),
  unacceptPlan: vi.fn(),
  discardPlan: vi.fn(),
  getReplanPreview: vi.fn(),
  replan: vi.fn(),
}))
vi.mock("@/lib/api/plates", () => ({
  confirmPlate: vi.fn(),
  unlockPlate: vi.fn(),
}))
vi.mock("@/lib/api/generation", () => ({
  generatePlan: vi.fn(),
  regeneratePlan: vi.fn(),
  getPlanRun: vi.fn(),
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
  listFoodMacros: vi.fn().mockResolvedValue({ foods: [] }),
  getFood: vi.fn(),
}))
const navigateMock = vi.hoisted(() => vi.fn())
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useNavigate: () => navigateMock,
}))
vi.mock("@/components/chat/ChatPanel", () => ({ ChatPanel: () => null }))
const toastMock = vi.hoisted(() =>
  Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn() })
)
vi.mock("@/lib/toast", () => ({ toast: toastMock, toastError: vi.fn() }))

import { getPlanRun, regeneratePlan } from "@/lib/api/generation"
import {
  acceptPlan,
  discardPlan,
  getPlan,
  getReplanPreview,
  restorePlan,
  unacceptPlan,
} from "@/lib/api/plan"
import { confirmPlate, unlockPlate } from "@/lib/api/plates"

import { PlanPage } from "./PlanPage"

interface ToastOptions {
  action: { label: string; onClick: () => void }
}

function lastToast() {
  const call = toastMock.mock.calls.at(-1)!
  return { message: call[0] as string, opts: call[1] as ToastOptions }
}

const WINDOW = { from: "2026-10-31", to: "2026-11-06" }
const CHILI_CLOSURE = { plate_ids: [1, 2, 3], session_ids: [7] }

/** mockDraftPlan after the Chili batch was confirmed. */
const confirmedPlan: Plan = {
  ...mockDraftPlan,
  cells: mockDraftPlan.cells.map((c) =>
    c.plate ? { ...c, plate: { ...c.plate, state: "confirmed" } } : c
  ),
  sessions: mockDraftPlan.sessions.map((s) => ({ ...s, state: "confirmed" })),
  issues: [],
  draft: null,
}

/** mockDraftPlan with rice as a side on Monday dinner (plate 3). */
const sidedPlan: Plan = {
  ...mockDraftPlan,
  cells: mockDraftPlan.cells.map((c) =>
    c.plate?.id === 3
      ? {
          ...c,
          plate: {
            ...c.plate,
            components: [
              ...c.plate.components,
              mockPlateComponent({
                id: 50,
                plate_id: 3,
                food_id: 5,
                food_name: "Rice",
                role: "side_starch",
                cook_session_id: undefined,
                sort_order: 1,
              }),
            ],
          } satisfies Plate,
        }
      : c
  ),
}

const cellButton = (name: RegExp) => screen.findByRole("button", { name })
const MONDAY_DINNER = /^Monday dinner, Chili/

beforeEach(async () => {
  vi.clearAllMocks()
  await i18n.changeLanguage("en")
  vi.mocked(getPlan).mockResolvedValue(mockDraftPlan)
  vi.mocked(regeneratePlan).mockResolvedValue({ run_id: 13 })
})

describe("review toolbar", () => {
  it("shows the draft summary, the run's notes and Accept and shop", async () => {
    renderWithRouter(<PlanPage />)
    expect(await screen.findByTestId("plan-summary")).toHaveTextContent(
      "Draft · 4 plates · 2 cook days"
    )
    expect(screen.getByTestId("plan-notes")).toHaveTextContent(
      "Cooks once on Sunday and eats leftovers on Monday."
    )
    expect(
      screen.getAllByRole("button", { name: "Accept and shop" })
    ).toHaveLength(2) // toolbar (≥768 px) and the phone DraftBar
    expect(screen.getByTestId("draft-bar")).toHaveTextContent(
      "Draft · 4 plates · 2 cook days"
    )
  })

  it("has no review actions without drafts", async () => {
    vi.mocked(getPlan).mockResolvedValue(mockPlan)
    renderWithRouter(<PlanPage />)
    expect(await screen.findByTestId("plan-summary")).toHaveTextContent(
      "3 plates, 1 cook day"
    )
    expect(
      screen.queryByRole("button", { name: "Accept and shop" })
    ).not.toBeInTheDocument()
    expect(screen.queryByTestId("plan-notes")).not.toBeInTheDocument()
  })

  it("Accept and shop confirms the window, goes to Shop and offers Undo", async () => {
    const user = userEvent.setup()
    vi.mocked(acceptPlan).mockResolvedValue(CHILI_CLOSURE)
    vi.mocked(unacceptPlan).mockResolvedValue(CHILI_CLOSURE)
    renderWithRouter(<PlanPage />)
    const [accept] = await screen.findAllByRole("button", {
      name: "Accept and shop",
    })
    await user.click(accept)
    await waitFor(() =>
      expect(navigateMock).toHaveBeenCalledWith({
        to: "/shop",
        search: WINDOW,
      })
    )
    expect(acceptPlan).toHaveBeenCalledWith(WINDOW.from, WINDOW.to)
    const { message, opts } = lastToast()
    expect(message).toBe("Plan accepted. 3 plates confirmed.")
    opts.action.onClick()
    await waitFor(() =>
      expect(unacceptPlan).toHaveBeenCalledWith(CHILI_CLOSURE)
    )
  })

  it("F2-AC6: Discard draft asks first, then discards with Undo", async () => {
    const user = userEvent.setup()
    const removed = { plates: [], sessions: [] }
    const snapshot = {
      ...removed,
      plates: [
        {
          date: "2026-11-01",
          slot_id: 3,
          eaters: 2,
          note: null,
          components: [],
          state: "draft" as const,
        },
      ],
    }
    vi.mocked(discardPlan).mockResolvedValue({ removed: snapshot })
    vi.mocked(restorePlan).mockResolvedValue({ plate_ids: [1] })
    renderWithRouter(<PlanPage />)
    await user.click(
      await screen.findByRole("button", { name: "More plan actions" })
    )
    await user.click(
      await screen.findByRole("menuitem", { name: "Discard draft" })
    )
    const dialog = await screen.findByRole("dialog", {
      name: "Discard this draft?",
    })
    expect(dialog).toHaveTextContent(
      "This deletes 3 draft plates. Confirmed plates stay."
    )
    // Cancel changes nothing.
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }))
    expect(discardPlan).not.toHaveBeenCalled()

    await user.click(screen.getByRole("button", { name: "More plan actions" }))
    await user.click(
      await screen.findByRole("menuitem", { name: "Discard draft" })
    )
    await user.click(
      within(
        await screen.findByRole("dialog", { name: "Discard this draft?" })
      ).getByRole("button", { name: "Discard draft" })
    )
    await waitFor(() =>
      expect(discardPlan).toHaveBeenCalledWith(WINDOW.from, WINDOW.to)
    )
    const { message, opts } = lastToast()
    expect(message).toBe("Draft discarded")
    opts.action.onClick()
    await waitFor(() => expect(restorePlan).toHaveBeenCalledWith(snapshot))
  })

  it("Regenerate week regenerates the window (D7)", async () => {
    const user = userEvent.setup()
    const { generatePlan } = await import("@/lib/api/generation")
    vi.mocked(generatePlan).mockResolvedValue({ run_id: 14 })
    renderWithRouter(<PlanPage />)
    await user.click(
      await screen.findByRole("button", { name: "More plan actions" })
    )
    await user.click(
      await screen.findByRole("menuitem", { name: "Regenerate week" })
    )
    await waitFor(() =>
      expect(generatePlan).toHaveBeenCalledWith({ start: WINDOW.from })
    )
    expect(navigateMock).toHaveBeenCalledWith({
      to: "/plan",
      search: { start: WINDOW.from, run: 14 },
    })
  })

  it("keeps a past window's drafts without review actions", async () => {
    vi.mocked(getPlan).mockResolvedValue({
      ...mockDraftPlan,
      window: { ...mockDraftPlan.window, today: "2026-12-01" },
    })
    const user = userEvent.setup()
    renderWithRouter(<PlanPage start="2026-10-31" />)
    await user.click(await cellButton(MONDAY_DINNER))
    await screen.findByRole("dialog", { name: "Chili" })
    expect(screen.queryByTestId("sheet-review")).not.toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Accept and shop" })
    ).not.toBeInTheDocument()
    expect(screen.queryByTestId("cell-quick-actions")).not.toBeInTheDocument()
  })
})

describe("plate sheet review actions", () => {
  it("F2-AC2: Confirm on a leftover padlocks the whole batch, and Undo unlocks it", async () => {
    const user = userEvent.setup()
    vi.mocked(confirmPlate).mockResolvedValue(CHILI_CLOSURE)
    vi.mocked(unlockPlate).mockResolvedValue(CHILI_CLOSURE)
    renderWithRouter(<PlanPage />)
    const grid = await screen.findByRole("grid")
    // Only the confirmed Tofu bowl has a padlock.
    expect(within(grid).getAllByTestId("cell-padlock")).toHaveLength(1)

    await user.click(await cellButton(MONDAY_DINNER))
    const sheet = await screen.findByRole("dialog", { name: "Chili" })
    vi.mocked(getPlan).mockResolvedValue(confirmedPlan)
    await user.click(within(sheet).getByRole("button", { name: "Confirm" }))

    expect(confirmPlate).toHaveBeenCalledWith(3)
    await waitFor(() =>
      expect(within(grid).getAllByTestId("cell-padlock")).toHaveLength(4)
    )
    for (const key of ["2026-11-01|3", "2026-11-02|2", "2026-11-02|3"]) {
      expect(
        grid.querySelector(
          `[data-cell-key="${key}"] [data-testid="cell-padlock"]`
        )
      ).not.toBeNull()
    }
    // The sheet closed so the Undo toast is in reach.
    expect(
      screen.queryByRole("dialog", { name: "Chili" })
    ).not.toBeInTheDocument()

    const { message, opts } = lastToast()
    expect(message).toBe("3 plates confirmed.")
    opts.action.onClick()
    await waitFor(() => expect(unlockPlate).toHaveBeenCalledWith(3))
  })

  it("F2-AC3: Unlock lists the batch closure; Cancel changes nothing", async () => {
    const user = userEvent.setup()
    vi.mocked(getPlan).mockResolvedValue(confirmedPlan)
    vi.mocked(unlockPlate).mockResolvedValue(CHILI_CLOSURE)
    renderWithRouter(<PlanPage />)
    await user.click(await cellButton(MONDAY_DINNER))
    const sheet = await screen.findByRole("dialog", { name: "Chili" })
    expect(sheet).toHaveTextContent(
      "Confirmed. Its ingredients may already be bought."
    )
    await user.click(within(sheet).getByRole("button", { name: "Unlock" }))

    const dialog = await screen.findByRole("dialog", {
      name: "Unlock this plate?",
    })
    expect(dialog).toHaveTextContent(
      "Unlocking also unlocks: Sunday dinner and Monday lunch."
    )
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }))
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: "Unlock this plate?" })
      ).not.toBeInTheDocument()
    )
    expect(unlockPlate).not.toHaveBeenCalled()

    // Unlock itself unlocks the closure through the plate.
    await user.click(within(sheet).getByRole("button", { name: "Unlock" }))
    await user.click(
      within(
        await screen.findByRole("dialog", { name: "Unlock this plate?" })
      ).getByRole("button", { name: "Unlock" })
    )
    await waitFor(() => expect(unlockPlate).toHaveBeenCalledWith(3))
    await waitFor(() =>
      expect(toastMock.success).toHaveBeenCalledWith("3 plates unlocked.")
    )
  })

  it("shows Why this? and the cell's issues", async () => {
    const user = userEvent.setup()
    vi.mocked(getPlan).mockResolvedValue({
      ...mockDraftPlan,
      issues: [
        ...mockDraftPlan.issues,
        {
          rule: "H8",
          code: "variety_repeat",
          severity: "violation",
          date: "2026-11-01",
          slot_id: 3,
          session_id: 7,
          params: { food_id: 42, last_eaten: "2026-10-20" },
        },
        {
          rule: "H7",
          code: "unknown_food",
          severity: "violation",
          date: "2026-11-01",
          slot_id: 3,
          params: { food_id: 99 },
        },
      ],
    })
    renderWithRouter(<PlanPage />)
    await user.click(await cellButton(/^Sunday dinner, Chili/))
    const sheet = await screen.findByRole("dialog", { name: "Chili" })
    expect(within(sheet).getByTestId("sheet-why")).toHaveTextContent(
      "Batch on Sunday feeds Monday lunch and dinner."
    )
    const issues = within(sheet).getByTestId("sheet-issues")
    expect(issues).toHaveTextContent(
      "2 cooking occasions on this day; your limit is 1."
    )
    expect(issues).toHaveTextContent("You had this recently, on Oct 20.")
    expect(issues).toHaveTextContent("Breaks a planning rule (H7).")
  })

  it("Replace, Different side and Regenerate day regenerate their scope", async () => {
    const user = userEvent.setup()
    vi.mocked(getPlan).mockResolvedValue(sidedPlan)
    renderWithRouter(<PlanPage />)

    const scopes = [
      ["Replace", { kind: "plate", plate_id: 3 }],
      ["Different side", { kind: "side", component_id: 50 }],
      ["Regenerate day", { kind: "day", date: "2026-11-02" }],
    ] as const
    for (const [action, scope] of scopes) {
      await user.click(await cellButton(MONDAY_DINNER))
      const sheet = await screen.findByRole("dialog", { name: "Chili" })
      await user.click(within(sheet).getByRole("button", { name: action }))
      await waitFor(() =>
        expect(regeneratePlan).toHaveBeenLastCalledWith({
          scope,
          start: WINDOW.from,
        })
      )
      // Regenerating closes the sheet; the board follows the run.
      await waitFor(() =>
        expect(
          screen.queryByRole("dialog", { name: "Chili" })
        ).not.toBeInTheDocument()
      )
    }
    expect(navigateMock).toHaveBeenCalledWith({
      to: "/plan",
      search: { start: WINDOW.from, run: 13 },
    })
  })

  it("a confirmed plate offers no Replace or Different side", async () => {
    const user = userEvent.setup()
    renderWithRouter(<PlanPage />)
    await user.click(await cellButton(/^Tuesday dinner, Tofu bowl/))
    const sheet = await screen.findByRole("dialog", { name: "Tofu bowl" })
    expect(
      within(sheet).queryByRole("button", { name: "Replace" })
    ).not.toBeInTheDocument()
    expect(
      within(sheet).getByRole("button", { name: "Regenerate day" })
    ).toBeInTheDocument()
  })
})

describe("keyboard (F2-AC7)", () => {
  it("R and S on a focused draft cell regenerate it; on a confirmed one nothing", async () => {
    const user = userEvent.setup()
    vi.mocked(getPlan).mockResolvedValue(sidedPlan)
    renderWithRouter(<PlanPage />)
    ;(await cellButton(MONDAY_DINNER)).focus()
    await user.keyboard("r")
    await waitFor(() =>
      expect(regeneratePlan).toHaveBeenLastCalledWith({
        scope: { kind: "plate", plate_id: 3 },
        start: WINDOW.from,
      })
    )
    ;(await cellButton(MONDAY_DINNER)).focus()
    await user.keyboard("S")
    await waitFor(() =>
      expect(regeneratePlan).toHaveBeenLastCalledWith({
        scope: { kind: "side", component_id: 50 },
        start: WINDOW.from,
      })
    )
    expect(regeneratePlan).toHaveBeenCalledTimes(2)
    ;(await cellButton(/^Tuesday dinner, Tofu bowl/)).focus()
    await user.keyboard("r")
    expect(regeneratePlan).toHaveBeenCalledTimes(2)
  })

  it("⌘/Ctrl+Enter accepts and shops", async () => {
    const user = userEvent.setup()
    vi.mocked(acceptPlan).mockResolvedValue(CHILI_CLOSURE)
    renderWithRouter(<PlanPage />)
    await screen.findByRole("grid")
    await user.keyboard("{Control>}{Enter}{/Control}")
    await waitFor(() =>
      expect(acceptPlan).toHaveBeenCalledWith(WINDOW.from, WINDOW.to)
    )
    await waitFor(() =>
      expect(navigateMock).toHaveBeenCalledWith({
        to: "/shop",
        search: WINDOW,
      })
    )
  })

  it("lists R, S and ⌘/Ctrl+Enter in the shortcuts", async () => {
    const user = userEvent.setup()
    renderWithRouter(<PlanPage />)
    await screen.findByRole("grid")
    await user.keyboard("?")
    const dialog = await screen.findByRole("dialog", {
      name: "Keyboard shortcuts",
    })
    expect(dialog).toHaveTextContent("Replace the meal")
    expect(dialog).toHaveTextContent("Choose a different side")
    expect(dialog).toHaveTextContent("Accept and shop")
  })
})

describe("generation progress (D35)", () => {
  it("shows the stage while the run is running, then announces it and drops ?run", async () => {
    vi.mocked(getPlanRun)
      .mockResolvedValueOnce(mockRun("running", "checking"))
      .mockResolvedValue(mockRun("ok"))
    renderWithRouter(<PlanPage start="2026-10-31" run={12} />)
    const bar = await screen.findByRole("progressbar", { name: "Planning" })
    await waitFor(() =>
      expect(bar).toHaveAttribute(
        "aria-valuetext",
        "Checking batches and keep days"
      )
    )
    expect(screen.getByTestId("generation-progress")).toHaveTextContent(
      "Checking batches and keep days"
    )
    // Review waits for the run.
    for (const b of screen.getAllByRole("button", {
      name: "Accept and shop",
    })) {
      expect(b).toBeDisabled()
    }

    await waitFor(
      () =>
        expect(toastMock).toHaveBeenCalledWith(
          "Plan regenerated. 1 confirmed plate kept."
        ),
      { timeout: 2000 }
    )
    expect(screen.queryByTestId("generation-progress")).not.toBeInTheDocument()
    expect(navigateMock).toHaveBeenCalledWith({
      to: "/plan",
      search: { start: "2026-10-31" },
      replace: true,
    })
  })

  it("re-attaches to the window's running generation without ?run", async () => {
    vi.mocked(getPlan).mockResolvedValue({
      ...mockDraftPlan,
      running_run_id: 12,
    })
    vi.mocked(getPlanRun).mockResolvedValue(mockRun("running", "choosing"))
    renderWithRouter(<PlanPage />)
    const bar = await screen.findByRole("progressbar", { name: "Planning" })
    await waitFor(() =>
      expect(bar).toHaveAttribute("aria-valuetext", "Choosing recipes")
    )
    expect(getPlanRun).toHaveBeenCalledWith(12)
  })

  it("announces a failed run with its error", async () => {
    vi.mocked(getPlanRun).mockResolvedValue(mockRun("error"))
    renderWithRouter(<PlanPage start="2026-10-31" run={12} />)
    await waitFor(() => expect(toastMock.error).toHaveBeenCalled())
    expect(screen.queryByTestId("generation-progress")).not.toBeInTheDocument()
  })

  it("gives up on a run it can't read: toast, no bar, ?run dropped", async () => {
    vi.mocked(getPlanRun).mockRejectedValue(new Error("404"))
    renderWithRouter(<PlanPage start="2026-10-31" run={12} />)
    await waitFor(() =>
      expect(toastMock.error).toHaveBeenCalledWith(
        "Planning didn't finish. Try again."
      )
    )
    expect(screen.queryByTestId("generation-progress")).not.toBeInTheDocument()
    expect(navigateMock).toHaveBeenCalledWith({
      to: "/plan",
      search: { start: "2026-10-31" },
      replace: true,
    })
    // Review is open again.
    for (const b of screen.getAllByRole("button", {
      name: "Accept and shop",
    })) {
      expect(b).toBeEnabled()
    }
  })

  it("regenerating waits while a run goes: no R/S, no quick actions, sheet buttons disabled", async () => {
    const user = userEvent.setup()
    vi.mocked(getPlan).mockResolvedValue({ ...sidedPlan, running_run_id: 12 })
    vi.mocked(getPlanRun).mockResolvedValue(mockRun("running", "choosing"))
    renderWithRouter(<PlanPage />)
    await screen.findByRole("progressbar", { name: "Planning" })
    expect(screen.queryByTestId("cell-quick-actions")).not.toBeInTheDocument()
    ;(await cellButton(MONDAY_DINNER)).focus()
    await user.keyboard("rs")
    expect(regeneratePlan).not.toHaveBeenCalled()

    await user.click(await cellButton(MONDAY_DINNER))
    const sheet = await screen.findByRole("dialog", { name: "Chili" })
    for (const name of ["Replace", "Different side", "Regenerate day"]) {
      expect(within(sheet).getByRole("button", { name })).toBeDisabled()
    }
  })
})

describe("on the anchored board", () => {
  it("R keeps the board on screen and the URL without start", async () => {
    const user = userEvent.setup()
    renderWithRouter(<PlanPage />, "/plan")
    ;(await cellButton(MONDAY_DINNER)).focus()
    await user.keyboard("r")
    await waitFor(() => expect(navigateMock).toHaveBeenCalled())
    const [{ to, search }] = navigateMock.mock.calls.at(-1) as [
      { to: string; search: (prev: object) => object },
    ]
    expect(to).toBe("/plan")
    expect(search({})).toEqual({ run: 13 })
    expect(
      screen.queryByRole("status", { name: "Loading the plan" })
    ).not.toBeInTheDocument()
    expect(screen.getByRole("grid")).toBeInTheDocument()
  })
})

describe("re-plan (F4)", () => {
  const menuEntry = () =>
    screen.queryByRole("menuitem", { name: "Re-plan from today" })

  async function openMenu(user: ReturnType<typeof userEvent.setup>) {
    await user.click(
      await screen.findByRole("button", { name: "More plan actions" })
    )
    await screen.findByRole("menuitem", { name: "Copy a previous week" })
  }

  beforeEach(() => {
    vi.mocked(getReplanPreview).mockResolvedValue({
      none_cells: [],
      confirmed_affected: [],
      spare: [],
    })
  })

  it("the current window's overflow opens the re-plan sheet from today", async () => {
    const user = userEvent.setup()
    vi.mocked(getPlan).mockResolvedValue(confirmedPlan)
    renderWithRouter(<PlanPage />)
    await openMenu(user)
    await user.click(menuEntry()!)
    const sheet = await screen.findByTestId("replan-sheet")
    expect(
      within(sheet).getByRole("heading", { name: "Re-plan from today" })
    ).toBeInTheDocument()
    await waitFor(() =>
      expect(getReplanPreview).toHaveBeenCalledWith(WINDOW.from, WINDOW.to, [])
    )
  })

  it.each([
    ["past", "2026-11-09"],
    ["future", "2026-10-20"],
  ])("F4-AC2: a %s window has no Re-plan from today", async (_, today) => {
    const user = userEvent.setup()
    vi.mocked(getPlan).mockResolvedValue({
      ...confirmedPlan,
      window: { ...confirmedPlan.window, today, is_current: false },
    })
    renderWithRouter(<PlanPage />)
    await openMenu(user)
    expect(menuEntry()).toBeNull()
  })

  it("Mark as No plate on a confirmed plate opens the re-plan with its cell", async () => {
    const user = userEvent.setup()
    vi.mocked(getPlan).mockResolvedValue(confirmedPlan)
    renderWithRouter(<PlanPage />)
    await user.click(await cellButton(MONDAY_DINNER))
    await user.click(
      await within(await screen.findByTestId("plan-sheet")).findByRole(
        "button",
        { name: "Mark as No plate" }
      )
    )
    const sheet = await screen.findByTestId("replan-sheet")
    expect(within(sheet).getByLabelText("Monday dinner")).toBeChecked()
    await waitFor(() =>
      expect(getReplanPreview).toHaveBeenCalledWith(WINDOW.from, WINDOW.to, [
        { date: "2026-11-02", slot_id: 3 },
      ])
    )
    // The plate sheet that opened it is gone: focus goes back to the cell.
    await user.click(within(sheet).getByRole("button", { name: "Cancel" }))
    await waitFor(() =>
      expect(document.activeElement).toHaveAccessibleName(MONDAY_DINNER)
    )
  })

  it("N on a confirmed cell opens the re-plan too; closing returns focus to the cell", async () => {
    const user = userEvent.setup()
    vi.mocked(getPlan).mockResolvedValue(confirmedPlan)
    renderWithRouter(<PlanPage />)
    ;(await cellButton(MONDAY_DINNER)).focus()
    await user.keyboard("n")
    const sheet = await screen.findByTestId("replan-sheet")
    expect(within(sheet).getByLabelText("Monday dinner")).toBeChecked()
    await user.click(within(sheet).getByRole("button", { name: "Cancel" }))
    await waitFor(() =>
      expect(document.activeElement).toHaveAccessibleName(MONDAY_DINNER)
    )
  })
})
