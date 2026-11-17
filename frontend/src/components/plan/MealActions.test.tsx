import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { AlsoChanged } from "@/lib/api/meal"
import i18n from "@/lib/i18n"
import type { CellOption } from "@/lib/plan/meal-actions"
import { mockPlan, mockSession } from "@/test/fixtures"
import { createHookWrapper } from "@/test/render"

const toastErrorMock = vi.hoisted(() => vi.fn())
vi.mock("@/lib/toast", () => ({
  toast: Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn() }),
  toastError: toastErrorMock,
}))

import {
  CellChoiceDialog,
  CellList,
  DayMenu,
  useAlsoChangedConfirm,
} from "./MealActions"

const slotName = (id: number) =>
  ({ 1: "Breakfast", 2: "Lunch", 3: "Dinner" })[id]!

beforeEach(async () => {
  vi.clearAllMocks()
  await i18n.changeLanguage("en")
})

const cooked: AlsoChanged = {
  session_id: mockSession.id,
  date: "2026-11-05",
  slot_id: 3,
  what: "cook_moved",
}

function Host({
  preview,
  commit,
}: {
  preview: () => Promise<AlsoChanged[]>
  commit: () => void
}) {
  const { confirm, dialog } = useAlsoChangedConfirm(mockPlan, slotName)
  return (
    <>
      <button onClick={() => confirm(preview, commit)}>go</button>
      {dialog}
    </>
  )
}

describe("useAlsoChangedConfirm", () => {
  it("commits at once when the dry run changes nothing else", async () => {
    const commit = vi.fn()
    render(<Host preview={async () => []} commit={commit} />, {
      wrapper: createHookWrapper(),
    })
    await userEvent.click(screen.getByRole("button", { name: "go" }))
    await waitFor(() => expect(commit).toHaveBeenCalledTimes(1))
    expect(screen.queryByTestId("also-changed")).not.toBeInTheDocument()
  })

  it("lists the other meals in words and commits only on Confirm", async () => {
    const commit = vi.fn()
    render(<Host preview={async () => [cooked]} commit={commit} />, {
      wrapper: createHookWrapper(),
    })
    await userEvent.click(screen.getByRole("button", { name: "go" }))
    const dialog = await screen.findByTestId("also-changed")
    expect(
      within(dialog).getByText("Chili will now be cooked Thu dinner")
    ).toBeInTheDocument()
    expect(commit).not.toHaveBeenCalled()
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Confirm" })
    )
    expect(commit).toHaveBeenCalledTimes(1)
    await waitFor(() =>
      expect(screen.queryByTestId("also-changed")).not.toBeInTheDocument()
    )
  })

  it("Cancel commits nothing", async () => {
    const commit = vi.fn()
    render(<Host preview={async () => [cooked]} commit={commit} />, {
      wrapper: createHookWrapper(),
    })
    await userEvent.click(screen.getByRole("button", { name: "go" }))
    await userEvent.click(await screen.findByRole("button", { name: "Cancel" }))
    expect(commit).not.toHaveBeenCalled()
  })

  it("a failed dry run shows the error and commits nothing", async () => {
    const commit = vi.fn()
    render(
      <Host
        preview={() => Promise.reject(new Error("boom"))}
        commit={commit}
      />,
      { wrapper: createHookWrapper() }
    )
    await userEvent.click(screen.getByRole("button", { name: "go" }))
    await waitFor(() => expect(toastErrorMock).toHaveBeenCalled())
    expect(commit).not.toHaveBeenCalled()
  })
})

describe("DayMenu", () => {
  it("offers Away all day, Plan this day and Regenerate day", async () => {
    const onAction = vi.fn()
    render(<DayMenu label="Monday" onAction={onAction} />)
    await userEvent.click(
      screen.getByRole("button", { name: "Actions for Monday" })
    )
    await userEvent.click(
      await screen.findByRole("menuitem", { name: "Away all day" })
    )
    expect(onAction).toHaveBeenLastCalledWith("away")

    await userEvent.click(
      screen.getByRole("button", { name: "Actions for Monday" })
    )
    await userEvent.click(
      await screen.findByRole("menuitem", { name: "Plan this day" })
    )
    expect(onAction).toHaveBeenLastCalledWith("plan")

    await userEvent.click(
      screen.getByRole("button", { name: "Actions for Monday" })
    )
    await userEvent.click(
      await screen.findByRole("menuitem", { name: "Regenerate day" })
    )
    expect(onAction).toHaveBeenLastCalledWith("regenerate")
  })
})

const free: CellOption = {
  key: "2026-11-03|2",
  date: "2026-11-03",
  slot_id: 2,
  plateId: null,
  name: null,
}
const taken: CellOption = {
  key: "2026-11-03|3",
  date: "2026-11-03",
  slot_id: 3,
  plateId: 9,
  name: "Lasagne",
}

describe("CellList", () => {
  it("says so when no cell is free", () => {
    render(<CellList cells={[]} slotName={slotName} onChoose={vi.fn()} />)
    expect(screen.getByText("No free meal this week.")).toBeInTheDocument()
  })

  it("lists each cell as a button with its occupant", async () => {
    const onChoose = vi.fn()
    render(
      <CellList cells={[free, taken]} slotName={slotName} onChoose={onChoose} />
    )
    expect(screen.getByText("Lasagne")).toBeInTheDocument()
    await userEvent.click(screen.getByRole("button", { name: /Tue lunch/ }))
    expect(onChoose).toHaveBeenCalledWith(free)
  })
})

describe("CellChoiceDialog", () => {
  function open(onChoose = vi.fn()) {
    render(
      <CellChoiceDialog
        open
        title="Move to…"
        cells={[free, taken]}
        slotName={slotName}
        onOpenChange={vi.fn()}
        onChoose={onChoose}
      />
    )
    return onChoose
  }

  it("a free cell is chosen at once", async () => {
    const onChoose = open()
    await userEvent.click(
      await screen.findByRole("button", { name: /Tue lunch/ })
    )
    expect(onChoose).toHaveBeenCalledWith(free)
  })

  it("a taken cell asks Swap or Replace", async () => {
    const onChoose = open()
    await userEvent.click(
      await screen.findByRole("button", { name: /Tue dinner/ })
    )
    expect(
      screen.getByText("Tue dinner already has Lasagne.")
    ).toBeInTheDocument()
    expect(onChoose).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole("button", { name: "Swap" }))
    expect(onChoose).toHaveBeenCalledWith(taken, "swap")
  })

  it("Replace drops the occupant; Cancel goes back to the list", async () => {
    const onChoose = open()
    await userEvent.click(
      await screen.findByRole("button", { name: /Tue dinner/ })
    )
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }))
    expect(
      screen.getByRole("button", { name: /Tue lunch/ })
    ).toBeInTheDocument()
    await userEvent.click(screen.getByRole("button", { name: /Tue dinner/ }))
    await userEvent.click(
      screen.getByRole("button", { name: "Replace Lasagne" })
    )
    expect(onChoose).toHaveBeenCalledWith(taken, "drop")
  })
})
