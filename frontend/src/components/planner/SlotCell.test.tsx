import { fireEvent, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, test, vi } from "vitest"

import "@/lib/i18n"
import type { Food } from "@/lib/api/foods"
import type { Plate } from "@/lib/api/plates"

import { SlotCell } from "./SlotCell"

function defaultProps(overrides: Partial<Parameters<typeof SlotCell>[0]> = {}) {
  return {
    day: 0,
    slotId: 1,
    plate: undefined as Plate | undefined,
    componentsById: new Map<number, Food>(),
    onAdd: vi.fn(),
    onDeletePlate: vi.fn(),
    onToggleFavorite: vi.fn(),
    onToggleSkip: vi.fn(),
    onRateLoved: vi.fn(),
    onRateDisliked: vi.fn(),
    ...overrides,
  }
}

describe("SlotCell — empty state", () => {
  test("renders inline skip button alongside the add affordance", () => {
    render(<SlotCell {...defaultProps()} />)

    expect(screen.getByTestId("slot-empty-add")).toBeInTheDocument()
    expect(screen.getByTestId("slot-empty-skip")).toBeInTheDocument()
  })

  test("clicking the inline skip button toggles skip without a note", async () => {
    const onToggleSkip = vi.fn()
    render(<SlotCell {...defaultProps({ onToggleSkip })} />)

    await userEvent.click(screen.getByTestId("slot-empty-skip"))

    expect(onToggleSkip).toHaveBeenCalledTimes(1)
    expect(onToggleSkip).toHaveBeenCalledWith()
  })

  test("right-click opens the note popover", async () => {
    render(<SlotCell {...defaultProps()} />)

    fireEvent.contextMenu(screen.getByTestId("slot-empty-skip"))

    expect(await screen.findByTestId("skip-note-popover")).toBeInTheDocument()
  })

  test("submitting the popover with text passes the note to onToggleSkip", async () => {
    const onToggleSkip = vi.fn()
    render(<SlotCell {...defaultProps({ onToggleSkip })} />)

    fireEvent.contextMenu(screen.getByTestId("slot-empty-skip"))
    const input = await screen.findByTestId("skip-note-input")
    await userEvent.type(input, "Eating at Joe's")
    await userEvent.click(screen.getByTestId("skip-note-save"))

    expect(onToggleSkip).toHaveBeenCalledWith("Eating at Joe's")
  })

  test("submitting the popover with empty text passes null", async () => {
    const onToggleSkip = vi.fn()
    render(<SlotCell {...defaultProps({ onToggleSkip })} />)

    fireEvent.contextMenu(screen.getByTestId("slot-empty-skip"))
    await screen.findByTestId("skip-note-input")
    await userEvent.click(screen.getByTestId("skip-note-save"))

    expect(onToggleSkip).toHaveBeenCalledWith(null)
  })

  test("pressing 'S' on the focused add button toggles skip", async () => {
    const onToggleSkip = vi.fn()
    render(<SlotCell {...defaultProps({ onToggleSkip })} />)

    const addBtn = screen.getByTestId("slot-empty-add")
    addBtn.focus()
    await userEvent.keyboard("s")

    expect(onToggleSkip).toHaveBeenCalledTimes(1)
    expect(onToggleSkip).toHaveBeenCalledWith()
  })

  test("clicking the add button calls onAdd, not onToggleSkip", async () => {
    const onAdd = vi.fn()
    const onToggleSkip = vi.fn()
    render(<SlotCell {...defaultProps({ onAdd, onToggleSkip })} />)

    await userEvent.click(screen.getByTestId("slot-empty-add"))

    expect(onAdd).toHaveBeenCalledTimes(1)
    expect(onToggleSkip).not.toHaveBeenCalled()
  })
})

describe("SlotCell — skipped state", () => {
  function skippedPlate(note: string | null = null): Plate {
    return {
      id: 999,
      slot_id: 1,
      date: "2026-05-02",
      note,
      skipped: true,
      components: [],
      created_at: "2026-05-02T10:00:00Z",
    }
  }

  test("renders the eating-out label", () => {
    render(<SlotCell {...defaultProps({ plate: skippedPlate() })} />)

    expect(screen.getByText(/eating out/i)).toBeInTheDocument()
  })

  test("renders the full note when present", () => {
    render(
      <SlotCell {...defaultProps({ plate: skippedPlate("Mom's birthday") })} />
    )

    expect(screen.getByTestId("slot-skip-note")).toHaveTextContent(
      "Mom's birthday"
    )
  })

  test("unmark button toggles skip without overriding the note", async () => {
    const onToggleSkip = vi.fn()
    render(
      <SlotCell
        {...defaultProps({ plate: skippedPlate("note"), onToggleSkip })}
      />
    )

    await userEvent.click(screen.getByTestId("slot-skip-unmark"))

    expect(onToggleSkip).toHaveBeenCalledTimes(1)
    expect(onToggleSkip).toHaveBeenCalledWith()
  })

  test("edit-note button opens the popover with the existing note", async () => {
    render(
      <SlotCell {...defaultProps({ plate: skippedPlate("Mom's birthday") })} />
    )

    await userEvent.click(screen.getByTestId("slot-skip-edit-note"))

    const input = (await screen.findByTestId(
      "skip-note-input"
    )) as HTMLInputElement
    expect(input.value).toBe("Mom's birthday")
  })

  test("saving the popover passes the trimmed note", async () => {
    const onToggleSkip = vi.fn()
    render(
      <SlotCell
        {...defaultProps({ plate: skippedPlate("old"), onToggleSkip })}
      />
    )

    await userEvent.click(screen.getByTestId("slot-skip-edit-note"))
    const input = (await screen.findByTestId(
      "skip-note-input"
    )) as HTMLInputElement
    await userEvent.clear(input)
    await userEvent.type(input, "  Joe's diner  ")
    await userEvent.click(screen.getByTestId("skip-note-save"))

    expect(onToggleSkip).toHaveBeenCalledWith("Joe's diner")
  })

  test("pressing 'S' while focused on the cell toggles skip", async () => {
    const onToggleSkip = vi.fn()
    const { container } = render(
      <SlotCell {...defaultProps({ plate: skippedPlate(), onToggleSkip })} />
    )
    const cell = container.querySelector(
      '[data-slot-state="skipped"]'
    ) as HTMLElement
    cell.focus()
    await userEvent.keyboard("s")

    expect(onToggleSkip).toHaveBeenCalledTimes(1)
    expect(onToggleSkip).toHaveBeenCalledWith()
  })

  test("pressing Delete while focused on the cell deletes the plate", async () => {
    const onDeletePlate = vi.fn()
    const { container } = render(
      <SlotCell {...defaultProps({ plate: skippedPlate(), onDeletePlate })} />
    )
    const cell = container.querySelector(
      '[data-slot-state="skipped"]'
    ) as HTMLElement
    cell.focus()
    await userEvent.keyboard("{Delete}")

    expect(onDeletePlate).toHaveBeenCalledTimes(1)
  })

  test("popover title for skipped-no-note state uses the existing-note key", async () => {
    render(<SlotCell {...defaultProps({ plate: skippedPlate(null) })} />)

    await userEvent.click(screen.getByTestId("slot-skip-edit-note"))

    // Reads "Add a note" — not "Skip with a note", since the slot is
    // already skipped and Save just adds the note.
    expect(await screen.findByText(/^add a note$/i)).toBeInTheDocument()
  })

  test("popover title for skipped-with-note state uses the edit key", async () => {
    render(
      <SlotCell {...defaultProps({ plate: skippedPlate("Mom's birthday") })} />
    )

    await userEvent.click(screen.getByTestId("slot-skip-edit-note"))

    expect(await screen.findByText(/edit skip note/i)).toBeInTheDocument()
  })

  test("action row stays in tab order with non-zero opacity", () => {
    render(<SlotCell {...defaultProps({ plate: skippedPlate() })} />)
    const editBtn = screen.getByTestId("slot-skip-edit-note")
    const unmarkBtn = screen.getByTestId("slot-skip-unmark")
    const deleteBtn = screen.getByTestId("slot-quick-delete")
    // None of the action-row buttons are aria-hidden or tabindex=-1 — they
    // remain reachable to keyboard users alongside the cell-level S /
    // Delete shortcuts.
    for (const btn of [editBtn, unmarkBtn, deleteBtn]) {
      expect(btn).not.toHaveAttribute("aria-hidden", "true")
      expect(btn.getAttribute("tabindex")).not.toBe("-1")
    }
    // The wrapping row is opacity-50 at rest, not opacity-0. unmarkBtn is
    // a direct child of the row (the edit button has a TooltipTrigger that
    // also lives inline with asChild, so the same parent applies).
    const row = unmarkBtn.parentElement
    expect(row?.className).toContain("opacity-50")
    expect(row?.className).not.toContain("opacity-0")
  })
})
