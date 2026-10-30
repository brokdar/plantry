import { fireEvent, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, test, vi } from "vitest"

import "@/lib/i18n"
import type { Food } from "@/lib/api/foods"
import type { AvailabilityCell } from "@/lib/api/availability"
import type { MacrosResponse, Plate } from "@/lib/api/plates"

import { SlotCell } from "./SlotCell"
import { mockPlate, mockPlateComponent } from "@/test/fixtures"

function defaultProps(overrides: Partial<Parameters<typeof SlotCell>[0]> = {}) {
  return {
    day: 0,
    slotId: 1,
    plate: undefined as Plate | undefined,
    componentsById: new Map<number, Food>(),
    onAdd: vi.fn(),
    onDeletePlate: vi.fn(),
    onToggleFavorite: vi.fn(),
    onMarkNoPlate: vi.fn(),
    onPlanMeal: vi.fn(),
    onEditNote: vi.fn(),
    onRateLoved: vi.fn(),
    onRateDisliked: vi.fn(),
    ...overrides,
  }
}

function cell(overrides: Partial<AvailabilityCell> = {}): AvailabilityCell {
  return {
    date: "2026-05-02",
    slot_id: 1,
    mode: "plan",
    source: "default",
    preset_id: null,
    preset_name: null,
    note: null,
    ...overrides,
  }
}

describe("SlotCell — empty state", () => {
  test("renders the Mark as No plate button alongside the add affordance", () => {
    render(<SlotCell {...defaultProps()} />)

    expect(screen.getByTestId("slot-empty-add")).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Mark as No plate" })
    ).toBeInTheDocument()
  })

  test("clicking Mark as No plate calls onMarkNoPlate without a note", async () => {
    const onMarkNoPlate = vi.fn()
    render(<SlotCell {...defaultProps({ onMarkNoPlate })} />)

    await userEvent.click(screen.getByTestId("slot-empty-no-plate"))

    expect(onMarkNoPlate).toHaveBeenCalledTimes(1)
    expect(onMarkNoPlate).toHaveBeenCalledWith()
  })

  test("right-click opens the note popover and passes the trimmed note", async () => {
    const onMarkNoPlate = vi.fn()
    render(<SlotCell {...defaultProps({ onMarkNoPlate })} />)

    fireEvent.contextMenu(screen.getByTestId("slot-empty-no-plate"))
    const input = await screen.findByTestId("no-plate-note-input")
    await userEvent.type(input, "  Dinner at Mum's ")
    await userEvent.click(screen.getByTestId("no-plate-note-save"))

    expect(onMarkNoPlate).toHaveBeenCalledWith("Dinner at Mum's")
  })

  test("submitting the popover with empty text passes null", async () => {
    const onMarkNoPlate = vi.fn()
    render(<SlotCell {...defaultProps({ onMarkNoPlate })} />)

    fireEvent.contextMenu(screen.getByTestId("slot-empty-no-plate"))
    await screen.findByTestId("no-plate-note-input")
    await userEvent.click(screen.getByTestId("no-plate-note-save"))

    expect(onMarkNoPlate).toHaveBeenCalledWith(null)
  })

  test("pressing 'N' on the focused add button marks No plate", async () => {
    const onMarkNoPlate = vi.fn()
    render(<SlotCell {...defaultProps({ onMarkNoPlate })} />)

    screen.getByTestId("slot-empty-add").focus()
    await userEvent.keyboard("n")

    expect(onMarkNoPlate).toHaveBeenCalledTimes(1)
  })

  test("clicking the add button calls onAdd, not onMarkNoPlate", async () => {
    const onAdd = vi.fn()
    const onMarkNoPlate = vi.fn()
    render(<SlotCell {...defaultProps({ onAdd, onMarkNoPlate })} />)

    await userEvent.click(screen.getByTestId("slot-empty-add"))

    expect(onAdd).toHaveBeenCalledTimes(1)
    expect(onMarkNoPlate).not.toHaveBeenCalled()
  })

  test("a routine cell without a plate shows its routine and still offers add", async () => {
    const onAdd = vi.fn()
    render(
      <SlotCell
        {...defaultProps({
          onAdd,
          availability: cell({
            mode: "routine",
            preset_id: 7,
            preset_name: "Oats",
          }),
        })}
      />
    )

    expect(screen.getByText("Routine · Oats")).toBeInTheDocument()
    await userEvent.click(screen.getByTestId("slot-empty-add"))
    expect(onAdd).toHaveBeenCalledTimes(1)
  })
})

describe("SlotCell — No plate state", () => {
  const noPlate = (note: string | null = null) =>
    cell({ mode: "none", source: "override", note })

  test("renders 'No plate' and the note", () => {
    const { container } = render(
      <SlotCell {...defaultProps({ availability: noPlate("Canteen") })} />
    )

    expect(container.querySelector('[data-slot-state="none"]')).not.toBeNull()
    expect(screen.getByText("No plate")).toBeInTheDocument()
    expect(screen.getByTestId("slot-no-plate-note")).toHaveTextContent(
      "Canteen"
    )
    expect(screen.queryByTestId("slot-empty-add")).not.toBeInTheDocument()
  })

  test("offers Plan this meal", async () => {
    const onPlanMeal = vi.fn()
    render(
      <SlotCell {...defaultProps({ availability: noPlate(), onPlanMeal })} />
    )

    await userEvent.click(
      screen.getByRole("button", { name: "Plan this meal" })
    )

    expect(onPlanMeal).toHaveBeenCalledTimes(1)
  })

  test("Edit note opens the popover with the note and saves the new one", async () => {
    const onEditNote = vi.fn()
    render(
      <SlotCell
        {...defaultProps({ availability: noPlate("old"), onEditNote })}
      />
    )

    await userEvent.click(screen.getByRole("button", { name: "Edit note" }))
    const input = (await screen.findByTestId(
      "no-plate-note-input"
    )) as HTMLInputElement
    expect(input.value).toBe("old")
    await userEvent.clear(input)
    await userEvent.type(input, "Canteen")
    await userEvent.click(screen.getByTestId("no-plate-note-save"))

    expect(onEditNote).toHaveBeenCalledWith("Canteen")
  })

  test("a plate in a none cell (default changed later) still renders with its actions", async () => {
    const onDeletePlate = vi.fn()
    const plate: Plate = mockPlate({
      id: 42,
      slot_id: 1,
      date: "2026-05-02",
      components: [mockPlateComponent({ id: 1, plate_id: 42, food_id: 7 })],
    })
    const food = { id: 7, name: "Bolognese" } as unknown as Food
    const { container } = render(
      <SlotCell
        {...defaultProps({
          plate,
          componentsById: new Map([[7, food]]),
          availability: cell({ mode: "none", source: "default" }),
          onDeletePlate,
        })}
      />
    )

    expect(
      container.querySelector('[data-slot-state="planned"]')
    ).not.toBeNull()
    expect(container.querySelector('[data-slot-state="none"]')).toBeNull()
    expect(screen.getByText("Bolognese")).toBeInTheDocument()
    expect(screen.queryByText("No plate")).not.toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Actions" })).toBeInTheDocument()
    await userEvent.click(screen.getByTestId("slot-quick-delete"))
    expect(onDeletePlate).toHaveBeenCalledTimes(1)
  })

  test("without a note the action reads Add a note", () => {
    render(<SlotCell {...defaultProps({ availability: noPlate() })} />)

    expect(
      screen.getByRole("button", { name: "Add a note" })
    ).toBeInTheDocument()
  })
})

describe("SlotCell — macros pill", () => {
  function plannedPlate(): Plate {
    return mockPlate({
      id: 42,
      slot_id: 1,
      date: "2026-05-02",
      components: [mockPlateComponent({ id: 1, plate_id: 42, food_id: 7 })],
    })
  }

  function macros(kcal: number): MacrosResponse {
    return { kcal, protein: 30, fat: 15, carbs: 60, fiber: 4, sodium: 200 }
  }

  const heroFood: Food = {
    id: 7,
    name: "Bolognese",
    kind: "composed",
    role: "main",
    image_path: null,
    favorite: false,
    cook_count: 0,
    last_cooked_at: null,
    created_at: "2026-05-01T00:00:00Z",
    updated_at: "2026-05-01T00:00:00Z",
    children: [],
    instructions: [],
    tags: [],
  } as unknown as Food

  test("renders kcal pill for planted cell when macros are provided", () => {
    render(
      <SlotCell
        {...defaultProps({
          plate: plannedPlate(),
          componentsById: new Map([[7, heroFood]]),
          macros: macros(620),
        })}
      />
    )
    const pill = screen.getByTestId("slot-cell-kcal")
    expect(pill).toBeInTheDocument()
    expect(pill.textContent).toMatch(/620/)
  })

  test("renders a placeholder while macros are loading (no layout shift)", () => {
    render(
      <SlotCell
        {...defaultProps({
          plate: plannedPlate(),
          componentsById: new Map([[7, heroFood]]),
        })}
      />
    )
    expect(
      screen.getByTestId("slot-cell-macros-placeholder")
    ).toBeInTheDocument()
    expect(screen.queryByTestId("slot-cell-kcal")).not.toBeInTheDocument()
  })

  test.each([
    [true, "h-[194px]"],
    [false, "h-[178px]"],
  ])("showMacros=%s sizes the cell %s", (showMacros, height) => {
    const { container } = render(
      <SlotCell
        {...defaultProps({
          plate: plannedPlate(),
          componentsById: new Map([[7, heroFood]]),
          macros: macros(620),
          showMacros,
        })}
      />
    )
    expect(container.querySelector("[data-slot-state='planned']")).toHaveClass(
      height
    )
  })
})
