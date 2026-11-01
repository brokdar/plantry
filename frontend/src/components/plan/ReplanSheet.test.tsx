import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { Plan, ReplanPreview } from "@/lib/api/plan"
import { mockPlan, mockRemovedBatch, mockSettingsList } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

// F4: the mid-week re-plan sheet (UX §5.10, D34). AC-1's window with today
// on Sunday 1 Nov: Monday's lunch and dinner eat the confirmed Chili.

vi.mock("@/lib/api/plan", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/plan")>()),
  getReplanPreview: vi.fn(),
  replan: vi.fn(),
}))
vi.mock("@/lib/api/settings", () => ({ listSettings: vi.fn() }))
const navigateMock = vi.hoisted(() => vi.fn())
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useNavigate: () => navigateMock,
}))
const toastMock = vi.hoisted(() =>
  Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn() })
)
vi.mock("@/lib/toast", () => ({ toast: toastMock, toastError: vi.fn() }))

import { getReplanPreview, replan } from "@/lib/api/plan"
import { listSettings } from "@/lib/api/settings"

import { ReplanSheet } from "./ReplanSheet"

const plan: Plan = {
  ...mockPlan,
  window: { ...mockPlan.window, today: "2026-11-01" },
}

const MONDAY = [1, 2, 3].map((slot_id) => ({ date: "2026-11-02", slot_id }))

const preview: ReplanPreview = {
  none_cells: [
    { date: "2026-11-02", slot_id: 2 },
    { date: "2026-11-02", slot_id: 3 },
  ],
  confirmed_affected: [
    { plate_id: 2, date: "2026-11-02", slot_id: 2, food_name: "Chili" },
    { plate_id: 3, date: "2026-11-02", slot_id: 3, food_name: "Chili" },
  ],
  spare: [
    {
      session_id: 1,
      food_name: "Chili",
      portions: 2,
      freezable: true,
      mutable: true,
    },
  ],
}

const EMPTY: ReplanPreview = {
  none_cells: [],
  confirmed_affected: [],
  spare: [],
}

function renderSheet(
  props: Partial<React.ComponentProps<typeof ReplanSheet>> = {}
) {
  const onOpenChange = vi.fn()
  renderWithRouter(
    <ReplanSheet
      open
      onOpenChange={onOpenChange}
      plan={plan}
      from="2026-11-01"
      initialCells={[]}
      {...props}
    />
  )
  return { onOpenChange }
}

async function sheet() {
  return screen.findByTestId("replan-sheet")
}

/** Any asked cell previews Monday's plates. */
function previewMonday() {
  vi.mocked(getReplanPreview).mockImplementation(async (_f, _t, asked) =>
    asked && asked.length > 0 ? preview : EMPTY
  )
}

/** The action waits for the preview of the current choice. */
async function clickWhenEnabled(s: HTMLElement, name: string) {
  const button = await within(s).findByRole("button", { name })
  await waitFor(() => expect(button).toBeEnabled())
  await userEvent.click(button)
}

describe("ReplanSheet", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(listSettings).mockResolvedValue(mockSettingsList([]))
    vi.mocked(getReplanPreview).mockResolvedValue(EMPTY)
    vi.mocked(replan).mockResolvedValue({ ...mockRemovedBatch, run_id: null })
  })

  it("lists the days from `from` to the window's end", async () => {
    renderSheet()
    const s = await sheet()
    expect(
      within(s).getByRole("heading", { name: "Re-plan from today" })
    ).toBeInTheDocument()
    const days = within(s).getAllByTestId("replan-day")
    expect(days).toHaveLength(6)
    expect(days[0]).toHaveTextContent("Sunday")
    expect(within(days[0]).getByLabelText("Sunday dinner")).toBeInTheDocument()
  })

  it("marking a whole day previews it and lists the change, the spare and the bought plates", async () => {
    previewMonday()
    renderSheet()
    const s = await sheet()
    await userEvent.click(
      within(within(s).getAllByTestId("replan-day")[1]).getByLabelText(
        "Whole day"
      )
    )
    await waitFor(() =>
      expect(getReplanPreview).toHaveBeenLastCalledWith(
        "2026-11-01",
        "2026-11-06",
        MONDAY
      )
    )
    expect(await within(s).findByTestId("replan-changes")).toHaveTextContent(
      "Monday is now No plate."
    )
    expect(within(s).getByTestId("replan-changes")).toHaveTextContent(
      "Chili has 2 spare portions."
    )
    expect(within(s).getByLabelText("Freeze 2 portions")).toBeChecked()
    const bought = within(s).getByTestId("replan-bought")
    expect(bought).toHaveTextContent("These were already bought for")
    expect(bought).toHaveTextContent("Monday lunch · Chili")
    expect(bought).toHaveTextContent("Monday dinner · Chili")
    expect(within(s).queryByRole("button", { name: "Re-plan" })).toBeNull()
  })

  it("Keep them closes and changes nothing", async () => {
    previewMonday()
    const { onOpenChange } = renderSheet({
      initialCells: [{ date: "2026-11-02", slot_id: 3 }],
    })
    const s = await sheet()
    await userEvent.click(
      await within(s).findByRole("button", { name: "Keep them" })
    )
    expect(onOpenChange).toHaveBeenCalledWith(false)
    expect(replan).not.toHaveBeenCalled()
  })

  it("Change them unlocks, freezes the spare and pins the heuristic without an assistant", async () => {
    previewMonday()
    const { onOpenChange } = renderSheet({
      initialCells: [{ date: "2026-11-02", slot_id: 3 }],
    })
    const s = await sheet()
    expect(within(s).getByLabelText("Monday dinner")).toBeChecked()
    await userEvent.click(within(s).getByLabelText("Monday lunch"))
    await userEvent.click(
      within(s).getByLabelText("Re-plan the rest of the week")
    )
    vi.mocked(replan).mockResolvedValue({ ...mockRemovedBatch, run_id: 21 })
    await clickWhenEnabled(s, "Change them")
    await waitFor(() =>
      expect(replan).toHaveBeenCalledWith({
        from: "2026-11-01",
        to: "2026-11-06",
        none_cells: [
          { date: "2026-11-02", slot_id: 3 },
          { date: "2026-11-02", slot_id: 2 },
        ],
        unlock_confirmed: true,
        spare: "freeze",
        regenerate: true,
        chooser: "heuristic",
      })
    )
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    // The board follows the started run (draft overlay, D35).
    expect(navigateMock).toHaveBeenCalledTimes(1)
    expect(navigateMock.mock.calls[0][0].search({})).toEqual({ run: 21 })
  })

  it("keeping the batch small sends spare keep; an assistant picks the chooser", async () => {
    vi.mocked(listSettings).mockResolvedValue(
      mockSettingsList([{ key: "ai.provider", value: "anthropic" }])
    )
    previewMonday()
    renderSheet({ initialCells: [{ date: "2026-11-02", slot_id: 3 }] })
    const s = await sheet()
    await userEvent.click(
      await within(s).findByLabelText("Cook 2 portions less")
    )
    await clickWhenEnabled(s, "Change them")
    await waitFor(() => expect(replan).toHaveBeenCalled())
    const input = vi.mocked(replan).mock.calls[0][0]
    expect(input.spare).toBe("keep")
    expect(input).not.toHaveProperty("chooser")
  })

  it("a batch that can still change but can't freeze cooks less", async () => {
    vi.mocked(getReplanPreview).mockResolvedValue({
      none_cells: [{ date: "2026-11-02", slot_id: 3 }],
      confirmed_affected: [],
      spare: [
        {
          session_id: 1,
          food_name: "Chili",
          portions: 2,
          freezable: false,
          mutable: true,
        },
      ],
    })
    renderSheet({ initialCells: [{ date: "2026-11-02", slot_id: 3 }] })
    const s = await sheet()
    expect(await within(s).findByTestId("replan-changes")).toHaveTextContent(
      "Chili: cook 2 portions less."
    )
    expect(within(s).queryByText("Spare portions")).toBeNull()
  })

  it("a cooked batch reports its unassigned portions and offers no choice", async () => {
    vi.mocked(getReplanPreview).mockResolvedValue({
      none_cells: [{ date: "2026-11-02", slot_id: 3 }],
      confirmed_affected: [],
      spare: [
        {
          session_id: 1,
          food_name: "Chili",
          portions: 1,
          freezable: false,
          mutable: false,
        },
      ],
    })
    renderSheet({ initialCells: [{ date: "2026-11-02", slot_id: 3 }] })
    const s = await sheet()
    expect(await within(s).findByTestId("replan-changes")).toHaveTextContent(
      "1 portion of Chili unassigned."
    )
    expect(within(s).queryByText("Spare portions")).toBeNull()
    expect(within(s).queryByTestId("replan-bought")).toBeNull()
    await clickWhenEnabled(s, "Re-plan")
    await waitFor(() =>
      expect(replan).toHaveBeenCalledWith(
        expect.objectContaining({
          unlock_confirmed: false,
          spare: "keep",
          regenerate: false,
        })
      )
    )
    expect(toastMock.success).toHaveBeenCalledWith(
      "Re-planned. 3 plates removed."
    )
  })

  it("No plate cells are shown ticked and can't be unticked", async () => {
    const cells = plan.cells.map((c) =>
      c.date === "2026-11-03" && c.slot_id === 1
        ? { ...c, mode: "none" as const }
        : c
    )
    renderSheet({ plan: { ...plan, cells } })
    const s = await sheet()
    const cell = within(s).getByLabelText("Tuesday breakfast")
    expect(cell).toBeChecked()
    expect(cell).toBeDisabled()
  })

  it("Re-plan waits for a choice", async () => {
    renderSheet()
    const s = await sheet()
    expect(within(s).getByRole("button", { name: "Re-plan" })).toBeDisabled()
    await userEvent.click(
      within(s).getByLabelText("Re-plan the rest of the week")
    )
    await waitFor(() =>
      expect(within(s).getByRole("button", { name: "Re-plan" })).toBeEnabled()
    )
  })

  it("titles a later start by its day", async () => {
    renderSheet({ from: "2026-11-03" })
    const s = await sheet()
    expect(
      within(s).getByRole("heading", { name: "Re-plan from Tuesday" })
    ).toBeInTheDocument()
    expect(within(s).getAllByTestId("replan-day")).toHaveLength(4)
  })
})
