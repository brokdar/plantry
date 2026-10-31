import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { Session } from "@/lib/api/sessions"
import i18n from "@/lib/i18n"
import { mockRemovedBatch, mockSession } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/sessions", () => ({
  getLeftoverCells: vi.fn(),
  addLeftovers: vi.fn(),
  updateSession: vi.fn(),
  deleteSession: vi.fn(),
}))
vi.mock("@/lib/api/plan", () => ({ restorePlan: vi.fn() }))
const toastMock = vi.hoisted(() =>
  Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn() })
)
vi.mock("@/lib/toast", () => ({ toast: toastMock, toastError: vi.fn() }))

import { restorePlan } from "@/lib/api/plan"
import {
  addLeftovers,
  deleteSession,
  getLeftoverCells,
  updateSession,
} from "@/lib/api/sessions"

import { BatchSection } from "./BatchSection"

const SLOTS: Record<number, string> = {
  1: "Breakfast",
  2: "Lunch",
  3: "Dinner",
}

/** A fresh Sunday-dinner Chili for two, not yet a batch. */
const single: Session = {
  ...mockSession,
  portions: 2,
  consumed: 2,
  links: [mockSession.links[0]],
  meals: 1,
}

async function renderSection(
  session: Session,
  componentId: number,
  opts: { freezing?: boolean; readOnly?: boolean } = {}
) {
  const onOpenCell = vi.fn()
  const onClose = vi.fn()
  renderWithRouter(
    <BatchSection
      session={session}
      componentId={componentId}
      colour={1}
      slotName={(id) => SLOTS[id]}
      freezing={opts.freezing ?? false}
      readOnly={opts.readOnly ?? false}
      onOpenCell={onOpenCell}
      onClose={onClose}
    />
  )
  // RouterProvider renders asynchronously.
  if (!opts.readOnly) await screen.findByTestId("batch-section")
  return { onOpenCell, onClose }
}

beforeEach(async () => {
  vi.clearAllMocks()
  await i18n.changeLanguage("en")
  vi.mocked(getLeftoverCells).mockImplementation(async (_id, count) => ({
    cells: [
      { date: "2026-11-02", slot_id: 2 },
      { date: "2026-11-02", slot_id: 3 },
      { date: "2026-11-03", slot_id: 2 },
    ].slice(0, count),
  }))
  vi.mocked(addLeftovers).mockResolvedValue(mockSession)
  vi.mocked(updateSession).mockResolvedValue(mockSession)
  vi.mocked(deleteSession).mockResolvedValue(mockRemovedBatch)
})

describe("BatchSection on a fresh single meal", () => {
  it("cooks as a batch of 3 meals with the suggested cells preselected", async () => {
    await renderSection(single, 9)
    await userEvent.click(
      screen.getByRole("button", { name: "Cook as a batch" })
    )
    await userEvent.click(
      screen.getByRole("button", { name: "Increase Meals" })
    )
    await waitFor(() => expect(getLeftoverCells).toHaveBeenCalledWith(7, 2))
    const boxes = await screen.findAllByRole("checkbox")
    expect(boxes).toHaveLength(2)
    boxes.forEach((b) => expect(b).toBeChecked())
    expect(screen.getByTestId("batch-coverage")).toHaveTextContent(
      "Covers Sun dinner, Mon lunch, Mon dinner"
    )

    await userEvent.click(screen.getByRole("button", { name: "Create batch" }))
    await waitFor(() =>
      expect(addLeftovers).toHaveBeenCalledWith(7, [
        { date: "2026-11-02", slot_id: 2 },
        { date: "2026-11-02", slot_id: 3 },
      ])
    )
  })

  it("leaves out an unticked cell", async () => {
    await renderSection(single, 9)
    await userEvent.click(
      screen.getByRole("button", { name: "Cook as a batch" })
    )
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "Mon lunch" })
    )
    expect(screen.getByTestId("batch-coverage")).toHaveTextContent(
      "Covers Sun dinner"
    )
    expect(screen.getByRole("button", { name: "Create batch" })).toBeDisabled()
  })

  it("Meals follows the ticked cells: unticking one lowers it", async () => {
    await renderSection(single, 9)
    await userEvent.click(
      screen.getByRole("button", { name: "Cook as a batch" })
    )
    await userEvent.click(
      screen.getByRole("button", { name: "Increase Meals" })
    )
    const meals = screen.getByRole("group", { name: "Meals" })
    expect(meals).toHaveTextContent("3")
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "Mon lunch" })
    )
    expect(meals).toHaveTextContent("2")
    expect(screen.getByTestId("batch-coverage")).toHaveTextContent(
      "Covers Sun dinner, Mon dinner"
    )
  })

  it("changing Meals keeps the user's unticks", async () => {
    await renderSection(single, 9)
    await userEvent.click(
      screen.getByRole("button", { name: "Cook as a batch" })
    )
    await userEvent.click(
      screen.getByRole("button", { name: "Increase Meals" })
    )
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "Mon lunch" })
    )
    const meals = screen.getByRole("group", { name: "Meals" })
    expect(meals).toHaveTextContent("2")

    // + adds the next suggested cell; Mon lunch stays unticked.
    await userEvent.click(
      screen.getByRole("button", { name: "Increase Meals" })
    )
    expect(meals).toHaveTextContent("3")
    expect(
      await screen.findByRole("checkbox", { name: "Tue lunch" })
    ).toBeChecked()
    expect(
      screen.getByRole("checkbox", { name: "Mon lunch" })
    ).not.toBeChecked()
    expect(screen.getByRole("checkbox", { name: "Mon dinner" })).toBeChecked()

    // − drops the last ticked cell; Mon lunch still stays unticked.
    await userEvent.click(
      screen.getByRole("button", { name: "Decrease Meals" })
    )
    expect(meals).toHaveTextContent("2")
    await waitFor(() =>
      expect(screen.queryByRole("checkbox", { name: "Tue lunch" })).toBeNull()
    )
    expect(
      screen.getByRole("checkbox", { name: "Mon lunch" })
    ).not.toBeChecked()
    expect(screen.getByRole("checkbox", { name: "Mon dinner" })).toBeChecked()
    await userEvent.click(screen.getByRole("button", { name: "Create batch" }))
    await waitFor(() =>
      expect(addLeftovers).toHaveBeenCalledWith(7, [
        { date: "2026-11-02", slot_id: 3 },
      ])
    )
  })

  it("starts again from 2 meals after Cancel", async () => {
    await renderSection(single, 9)
    await userEvent.click(
      screen.getByRole("button", { name: "Cook as a batch" })
    )
    await userEvent.click(
      screen.getByRole("button", { name: "Increase Meals" })
    )
    expect(screen.getByRole("group", { name: "Meals" })).toHaveTextContent("3")
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }))
    await userEvent.click(
      screen.getByRole("button", { name: "Cook as a batch" })
    )
    expect(screen.getByRole("group", { name: "Meals" })).toHaveTextContent("2")
  })

  it("is hidden in a past window", async () => {
    await renderSection(single, 9, { readOnly: true })
    expect(
      screen.queryByRole("button", { name: "Cook as a batch" })
    ).not.toBeInTheDocument()
  })
})

describe("BatchSection on the cook cell of a batch", () => {
  it("sets the portions and shows the spare ones", async () => {
    await renderSection({ ...mockSession, portions: 8, surplus: 2 }, 9)
    expect(screen.getByTestId("batch-coverage")).toHaveTextContent(
      "Covers Sun dinner, Mon lunch, Mon dinner · 2 spare"
    )
    expect(screen.getByText("Use by Wed, Nov 4")).toBeVisible()
    // While the edit is in flight the text follows the stepper live.
    vi.mocked(updateSession).mockReturnValue(new Promise(() => {}))
    await userEvent.click(
      screen.getByRole("button", { name: "Increase Portions to cook" })
    )
    expect(screen.getByTestId("batch-coverage")).toHaveTextContent("3 spare")
    await waitFor(() =>
      expect(updateSession).toHaveBeenCalledWith(7, {
        portions: 9,
        frozen_portions: 0,
        note: null,
      })
    )
  })

  it("shows the freeze stepper only with freezing on", async () => {
    await renderSection(mockSession, 9)
    expect(
      screen.queryByRole("group", { name: "Portions to freeze" })
    ).not.toBeInTheDocument()
  })

  it("freezes portions when freezing is on", async () => {
    await renderSection({ ...mockSession, portions: 8 }, 9, { freezing: true })
    await userEvent.click(
      screen.getByRole("button", { name: "Increase Portions to freeze" })
    )
    await waitFor(() =>
      expect(updateSession).toHaveBeenCalledWith(7, {
        portions: 8,
        frozen_portions: 1,
        note: null,
      })
    )
  })

  it("asks before Remove batch, then closes the sheet and offers Undo", async () => {
    const { onClose } = await renderSection(mockSession, 9)
    await userEvent.click(screen.getByRole("button", { name: "Remove batch" }))
    const dialog = await screen.findByRole("dialog", {
      name: "Remove this batch?",
    })
    expect(dialog).toHaveTextContent("2 other plates")
    expect(deleteSession).not.toHaveBeenCalled()
    await userEvent.click(
      screen.getAllByRole("button", { name: "Remove batch" }).at(-1)!
    )
    await waitFor(() => expect(deleteSession).toHaveBeenCalledWith(7))
    await waitFor(() => expect(onClose).toHaveBeenCalled())
    await waitFor(() => expect(toastMock).toHaveBeenCalled())
    const [message, opts] = toastMock.mock.calls.at(-1)!
    expect(message).toBe("Batch removed")
    ;(opts as { action: { onClick: () => void } }).action.onClick()
    await waitFor(() =>
      expect(restorePlan).toHaveBeenCalledWith(mockRemovedBatch.removed)
    )
  })
})

describe("BatchSection on a leftover", () => {
  it("says where it comes from and opens the cook cell", async () => {
    const { onOpenCell } = await renderSection(mockSession, 10)
    expect(screen.getByText("From Sun · Chili (2 of 3)")).toBeVisible()
    await userEvent.click(screen.getByRole("button", { name: "Open batch" }))
    expect(onOpenCell).toHaveBeenCalledWith("2026-11-01|3")
  })
})
