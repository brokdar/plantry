import { screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useState } from "react"
import { describe, expect, it, vi } from "vitest"

import {
  mockAvailabilityColumns,
  mockAvailabilityPresets,
  mockAvailabilitySlots,
} from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

import {
  AvailabilityGrid,
  type AvailabilityCellValue,
  type AvailabilityGridValue,
} from "./AvailabilityGrid"

type GridOverrides = Partial<React.ComponentProps<typeof AvailabilityGrid>>

/** Controlled harness: applies onChange / onBulkChange to local state. */
function Harness({
  initial = {},
  spy,
  ...props
}: GridOverrides & {
  initial?: AvailabilityGridValue
  spy?: (colKey: string, slotId: number, next: AvailabilityCellValue) => void
}) {
  const [value, setValue] = useState<AvailabilityGridValue>(initial)
  return (
    <AvailabilityGrid
      label="Default week"
      columns={mockAvailabilityColumns}
      slots={mockAvailabilitySlots}
      presets={mockAvailabilityPresets}
      value={value}
      onChange={(colKey, slotId, next) => {
        spy?.(colKey, slotId, next)
        setValue((v) => ({ ...v, [`${colKey}:${slotId}`]: next }))
      }}
      {...props}
    />
  )
}

const cell = (name: string) => screen.findByRole("button", { name })

describe("AvailabilityGrid", () => {
  it("renders a grid with column and row headers", async () => {
    renderWithRouter(<Harness />)
    const grid = await screen.findByRole("grid", { name: "Default week" })
    expect(
      within(grid)
        .getAllByRole("columnheader")
        .map((h) => h.textContent)
    ).toEqual(expect.arrayContaining(["Monday", "Sunday"]))
    expect(
      within(grid).getByRole("rowheader", { name: "Lunch" })
    ).toBeInTheDocument()
    expect(within(grid).getAllByRole("gridcell")).toHaveLength(21)
  })

  it("scrolls inside a box that contains its sr-only labels", async () => {
    // Without `relative`, the labels position against the page and widen it
    // on a phone (jsdom has no layout, so this guards the class).
    renderWithRouter(<Harness />)
    const grid = await screen.findByRole("grid", { name: "Default week" })
    expect(grid.parentElement).toHaveClass("relative", "overflow-x-auto")
  })

  it("keeps its minimum width at every breakpoint and wraps state words", async () => {
    // jsdom has no layout: guard the classes that keep cells readable. The
    // grid scrolls in its box below its minimum width instead of shrinking
    // cells to "Pla…".
    renderWithRouter(<Harness initial={{ "0:2": { mode: "none" } }} />)
    const grid = await screen.findByRole("grid", { name: "Default week" })
    expect(grid).toHaveClass("min-w-[32rem]", "table-fixed")
    expect(grid.className).not.toMatch(/\b\w+:min-w-0\b/)
    const state = within(await cell("Monday lunch, No plate")).getByText(
      "No plate"
    )
    expect(state).not.toHaveClass("truncate")
  })

  it("names every cell in full and shows the state as text", async () => {
    renderWithRouter(
      <Harness
        initial={{
          "0:2": { mode: "none" },
          "1:1": { mode: "routine", presetId: 7 },
        }}
      />
    )
    expect(await cell("Monday breakfast, Plan")).toHaveTextContent("Plan")
    expect(await cell("Monday lunch, No plate")).toHaveTextContent("No plate")
    const routine = await cell("Tuesday breakfast, Routine · Overnight oats")
    expect(routine).toHaveTextContent("Routine")
    expect(routine).toHaveTextContent("Overnight oats")
  })

  it("cycles Plan → No plate → Routine (via picker) → Plan", async () => {
    const user = userEvent.setup()
    const spy = vi.fn()
    renderWithRouter(<Harness spy={spy} />)

    await user.click(await cell("Monday lunch, Plan"))
    expect(spy).toHaveBeenLastCalledWith("0", 2, { mode: "none" })

    await user.click(await cell("Monday lunch, No plate"))
    // Choosing Routine opens the picker; nothing changes until a pick.
    expect(spy).toHaveBeenCalledTimes(1)
    const picker = await screen.findByRole("dialog", { name: "Pick a routine" })
    await user.click(
      within(picker).getByRole("button", { name: "Bread and soup" })
    )
    expect(spy).toHaveBeenLastCalledWith("0", 2, {
      mode: "routine",
      presetId: 8,
    })

    await user.click(await cell("Monday lunch, Routine · Bread and soup"))
    expect(spy).toHaveBeenLastCalledWith("0", 2, { mode: "plan" })
    expect(await cell("Monday lunch, Plan")).toBeInTheDocument()
  })

  it("Escape on the picker skips Routine and moves on to Plan", async () => {
    const user = userEvent.setup()
    const spy = vi.fn()
    renderWithRouter(
      <Harness spy={spy} initial={{ "0:2": { mode: "none" } }} />
    )

    await user.click(await cell("Monday lunch, No plate"))
    await screen.findByRole("dialog", { name: "Pick a routine" })
    await user.keyboard("{Escape}")

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument()
    expect(spy).toHaveBeenCalledTimes(1)
    expect(spy).toHaveBeenLastCalledWith("0", 2, { mode: "plan" })
    expect(await cell("Monday lunch, Plan")).toHaveFocus()
  })

  it("the picker's cancel button also moves on to Plan", async () => {
    const user = userEvent.setup()
    const spy = vi.fn()
    renderWithRouter(
      <Harness spy={spy} initial={{ "0:2": { mode: "none" } }} />
    )

    await user.click(await cell("Monday lunch, No plate"))
    const picker = await screen.findByRole("dialog", { name: "Pick a routine" })
    await user.click(within(picker).getByRole("button", { name: "Cancel" }))

    expect(spy).toHaveBeenCalledTimes(1)
    expect(spy).toHaveBeenLastCalledWith("0", 2, { mode: "plan" })
    expect(await cell("Monday lunch, Plan")).toBeInTheDocument()
  })

  it("picking a routine is the only change the picker makes", async () => {
    const user = userEvent.setup()
    const spy = vi.fn()
    renderWithRouter(
      <Harness spy={spy} initial={{ "0:2": { mode: "none" } }} />
    )
    await user.click(await cell("Monday lunch, No plate"))
    const picker = await screen.findByRole("dialog", { name: "Pick a routine" })
    await user.click(
      within(picker).getByRole("button", { name: "Overnight oats" })
    )
    expect(spy).toHaveBeenCalledTimes(1)
    expect(spy).toHaveBeenLastCalledWith("0", 2, {
      mode: "routine",
      presetId: 7,
    })
  })

  it("without routines, No plate cycles straight back to Plan", async () => {
    const user = userEvent.setup()
    const spy = vi.fn()
    renderWithRouter(<Harness spy={spy} presets={[]} />)

    await user.click(await cell("Monday lunch, Plan"))
    const none = await cell("Monday lunch, No plate")
    expect(none).not.toHaveAttribute("aria-haspopup")
    await user.click(none)

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument()
    expect(spy.mock.calls).toEqual([
      ["0", 2, { mode: "none" }],
      ["0", 2, { mode: "plan" }],
    ])
    expect(await cell("Monday lunch, Plan")).toBeInTheDocument()
  })

  it("emits onChange with the column key, slot id and next value", async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    renderWithRouter(
      <AvailabilityGrid
        label="Default week"
        columns={mockAvailabilityColumns}
        slots={mockAvailabilitySlots}
        presets={mockAvailabilityPresets}
        value={{ "6:3": { mode: "routine", presetId: 7 } }}
        onChange={onChange}
      />
    )
    await user.click(await cell("Sunday dinner, Routine · Overnight oats"))
    expect(onChange).toHaveBeenCalledWith("6", 3, { mode: "plan" })
    await user.click(await cell("Friday breakfast, Plan"))
    expect(onChange).toHaveBeenCalledWith("4", 1, { mode: "none" })
  })

  it("moves between cells with arrow keys and cycles with Enter and Space", async () => {
    const user = userEvent.setup()
    const spy = vi.fn()
    renderWithRouter(<Harness spy={spy} />)

    const first = await cell("Monday breakfast, Plan")
    // Roving tabindex: one tab stop for the whole grid.
    expect(first).toHaveAttribute("tabindex", "0")
    expect(await cell("Tuesday breakfast, Plan")).toHaveAttribute(
      "tabindex",
      "-1"
    )

    await user.tab()
    expect(first).toHaveFocus()
    await user.keyboard("{ArrowRight}")
    expect(await cell("Tuesday breakfast, Plan")).toHaveFocus()
    await user.keyboard("{ArrowDown}")
    expect(await cell("Tuesday lunch, Plan")).toHaveFocus()
    await user.keyboard("{ArrowLeft}")
    expect(await cell("Monday lunch, Plan")).toHaveFocus()
    await user.keyboard("{ArrowUp}{ArrowUp}{ArrowLeft}")
    expect(first).toHaveFocus()

    await user.keyboard("{Enter}")
    expect(spy).toHaveBeenLastCalledWith("0", 1, { mode: "none" })
    expect(await cell("Monday breakfast, No plate")).toHaveFocus()

    await user.keyboard("{ArrowRight} ")
    expect(spy).toHaveBeenLastCalledWith("1", 1, { mode: "none" })
  })

  it("sticky header cells match the surface", async () => {
    const { unmount } = renderWithRouter(<Harness />)
    expect(await screen.findByRole("rowheader", { name: "Lunch" })).toHaveClass(
      "bg-ground"
    )
    unmount()

    renderWithRouter(<Harness surface="panel" />)
    const rowHeader = await screen.findByRole("rowheader", { name: "Lunch" })
    expect(rowHeader).toHaveClass("bg-panel")
    expect(rowHeader).not.toHaveClass("bg-ground")
    expect(screen.getByRole("columnheader", { name: "Meal" })).toHaveClass(
      "bg-panel"
    )
  })

  it("uses the full column name for headers and cells", async () => {
    renderWithRouter(
      <Harness columns={[{ key: "0", label: "Mon", fullLabel: "Monday" }]} />
    )
    expect(
      await screen.findByRole("columnheader", { name: "Monday" })
    ).toBeInTheDocument()
    expect(await cell("Monday lunch, Plan")).toBeInTheDocument()
  })

  it("disables every cell when disabled", async () => {
    renderWithRouter(<Harness disabled />)
    expect(await cell("Monday breakfast, Plan")).toBeDisabled()
  })

  it("renders no column menu without onBulkChange", async () => {
    renderWithRouter(<Harness />)
    await screen.findByRole("grid")
    expect(
      screen.queryByRole("button", { name: "Monday actions" })
    ).not.toBeInTheDocument()
  })

  it("a column bulk action sets every slot of that column", async () => {
    const user = userEvent.setup()
    const onBulkChange = vi.fn()
    renderWithRouter(
      <AvailabilityGrid
        label="Default week"
        columns={mockAvailabilityColumns}
        slots={mockAvailabilitySlots}
        presets={mockAvailabilityPresets}
        value={{}}
        onChange={vi.fn()}
        onBulkChange={onBulkChange}
      />
    )
    await user.click(
      await screen.findByRole("button", { name: "Wednesday actions" })
    )
    await user.click(
      await screen.findByRole("menuitem", { name: "No plate all day" })
    )
    expect(onBulkChange).toHaveBeenCalledWith({
      "2:1": { mode: "none" },
      "2:2": { mode: "none" },
      "2:3": { mode: "none" },
    })

    await user.click(
      await screen.findByRole("button", { name: "Wednesday actions" })
    )
    await user.click(
      await screen.findByRole("menuitem", { name: "Plan all day" })
    )
    expect(onBulkChange).toHaveBeenLastCalledWith({
      "2:1": { mode: "plan" },
      "2:2": { mode: "plan" },
      "2:3": { mode: "plan" },
    })
  })
})
