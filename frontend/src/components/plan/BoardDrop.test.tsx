import { act, render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { DragEndEvent } from "@dnd-kit/core"
import { beforeEach, describe, expect, it, vi } from "vitest"

import i18n from "@/lib/i18n"
import { buildBoard, type BoardCell } from "@/lib/plan/board"
import { edited, mockPlan, mockPlate } from "@/test/fixtures"
import { createHookWrapper } from "@/test/render"

// jsdom cannot drag: DndContext hands its onDragEnd to the test, which
// calls it with the events the sensors would produce.
const dnd = vi.hoisted(() => ({
  onDragEnd: null as null | ((e: DragEndEvent) => void),
}))
vi.mock("@dnd-kit/core", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@dnd-kit/core")>()),
  DndContext: ({
    onDragEnd,
    children,
  }: {
    onDragEnd: (e: DragEndEvent) => void
    children: React.ReactNode
  }) => {
    dnd.onDragEnd = onDragEnd
    return children
  },
}))
vi.mock("@/lib/api/meal", () => ({ movePlate: vi.fn(), revertPlan: vi.fn() }))
vi.mock("@/lib/api/plan", () => ({ restorePlan: vi.fn() }))
vi.mock("@/lib/dates", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/dates")>()),
  todayISO: () => "2026-10-31",
}))
const toastMock = vi.hoisted(() =>
  Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn() })
)
vi.mock("@/lib/toast", () => ({ toast: toastMock, toastError: vi.fn() }))

import { movePlate } from "@/lib/api/meal"

import { Board } from "./Board"

const slotName = (id: number) => ["", "Breakfast", "Lunch", "Dinner"][id]
const board = buildBoard(mockPlan, i18n)
const cells = board.days.flatMap((d) => d.cells)
const cellAt = (key: string) => cells.find((c) => c.key === key)!

const SUNDAY_DINNER = "2026-11-01|3" // plate 1, cooks the batch
const MONDAY_LUNCH = "2026-11-02|2" // plate 2, taken
const MONDAY_BREAKFAST = "2026-11-02|1" // free

function drop(from: BoardCell, to: BoardCell) {
  act(() =>
    dnd.onDragEnd!({
      active: { data: { current: { cell: from } } },
      over: {
        data: { current: { cell: to } },
        rect: { left: 100, top: 100, width: 80, height: 40 },
      },
    } as unknown as DragEndEvent)
  )
}

function renderBoard() {
  return render(
    <Board
      plan={mockPlan}
      slotName={slotName}
      board={board}
      targets={mockPlan.targets}
      label="Meal plan"
      onOpenCell={vi.fn()}
      onMarkNoPlate={vi.fn()}
    />,
    { wrapper: createHookWrapper() }
  )
}

beforeEach(async () => {
  vi.clearAllMocks()
  await i18n.changeLanguage("en")
  vi.mocked(movePlate).mockResolvedValue(
    edited({ plates: [mockPlate()], session_ids: [] })
  )
})

describe("Board drop", () => {
  it("a drop on an empty cell previews, moves and offers Undo", async () => {
    renderBoard()
    drop(cellAt(MONDAY_LUNCH), cellAt(MONDAY_BREAKFAST))
    await waitFor(() =>
      expect(movePlate).toHaveBeenCalledWith(
        2,
        { date: "2026-11-02", slot_id: 1, occupant: undefined },
        true
      )
    )
    await waitFor(() => expect(movePlate).toHaveBeenCalledTimes(2))
    expect(movePlate).toHaveBeenLastCalledWith(2, {
      date: "2026-11-02",
      slot_id: 1,
      occupant: undefined,
    })
    await waitFor(() =>
      expect(toastMock).toHaveBeenCalledWith("Meal moved", expect.anything())
    )
  })

  it("a drop on a taken cell opens the menu instead of failing", async () => {
    renderBoard()
    drop(cellAt(SUNDAY_DINNER), cellAt(MONDAY_LUNCH))
    const menu = await screen.findByTestId("occupied-menu")
    expect(movePlate).not.toHaveBeenCalled()
    for (const name of ["Swap", /^Move Chili to/, /^Replace Chili/, "Cancel"]) {
      expect(within(menu).getByRole("menuitem", { name })).toBeInTheDocument()
    }
  })

  it("Swap sends occupant swap", async () => {
    renderBoard()
    drop(cellAt(SUNDAY_DINNER), cellAt(MONDAY_LUNCH))
    await userEvent.click(await screen.findByRole("menuitem", { name: "Swap" }))
    await waitFor(() =>
      expect(movePlate).toHaveBeenLastCalledWith(1, {
        date: "2026-11-02",
        slot_id: 2,
        occupant: "swap",
      })
    )
    await waitFor(() =>
      expect(toastMock).toHaveBeenCalledWith("Meals swapped", expect.anything())
    )
  })

  it("Replace sends occupant drop", async () => {
    renderBoard()
    drop(cellAt(SUNDAY_DINNER), cellAt(MONDAY_LUNCH))
    await userEvent.click(
      await screen.findByRole("menuitem", { name: /^Replace Chili/ })
    )
    await waitFor(() =>
      expect(movePlate).toHaveBeenLastCalledWith(1, {
        date: "2026-11-02",
        slot_id: 2,
        occupant: "drop",
      })
    )
  })

  it("Move X to… moves the occupant to a free cell it picks", async () => {
    renderBoard()
    drop(cellAt(SUNDAY_DINNER), cellAt(MONDAY_LUNCH))
    await userEvent.click(
      await screen.findByRole("menuitem", { name: /^Move Chili to/ })
    )
    await userEvent.click(
      await screen.findByRole("menuitem", { name: "Sat lunch" })
    )
    await waitFor(() =>
      expect(movePlate).toHaveBeenLastCalledWith(1, {
        date: "2026-11-02",
        slot_id: 2,
        occupant: { date: "2026-10-31", slot_id: 2 },
      })
    )
  })

  it("Cancel moves nothing", async () => {
    renderBoard()
    drop(cellAt(SUNDAY_DINNER), cellAt(MONDAY_LUNCH))
    await userEvent.click(
      await screen.findByRole("menuitem", { name: "Cancel" })
    )
    await waitFor(() =>
      expect(screen.queryByTestId("occupied-menu")).not.toBeInTheDocument()
    )
    expect(movePlate).not.toHaveBeenCalled()
  })
})
