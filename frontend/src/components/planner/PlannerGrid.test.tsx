import { screen } from "@testing-library/react"
import type { DndContextProps, DragEndEvent } from "@dnd-kit/core"
import userEvent from "@testing-library/user-event"
import { addDays, format, subDays } from "date-fns"
import { beforeEach, describe, expect, test, vi } from "vitest"

import { renderWithRouter } from "@/test/render"
import type { AvailabilityCell } from "@/lib/api/availability"
import type { Plate } from "@/lib/api/plates"
import type { TimeSlot } from "@/lib/api/slots"

import { PlannerGrid, type PlannerDay } from "./PlannerGrid"

const mocks = vi.hoisted(() => ({
  updatePlate: vi.fn(),
  markNoPlate: vi.fn(),
  planMeal: vi.fn(),
  editNote: vi.fn(),
  dndProps: null as DndContextProps | null,
}))

// Capture the DndContext handlers so a drop can be fired without simulating
// pointer geometry in jsdom.
vi.mock("@dnd-kit/core", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@dnd-kit/core")>()
  return {
    ...actual,
    DndContext: (props: DndContextProps) => {
      mocks.dndProps = props
      return <actual.DndContext {...props} />
    },
  }
})

// Mock all query hooks so PlannerGrid renders without a live server.
vi.mock("@/lib/queries/foods", () => ({
  useFoods: vi.fn(() => ({ data: { items: [] } })),
  useSetFoodFavorite: vi.fn(() => ({ mutateAsync: vi.fn() })),
}))
vi.mock("@/lib/queries/plates", () => ({
  usePlannerUpdatePlate: vi.fn(() => ({ mutateAsync: mocks.updatePlate })),
  useDeletePlate: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useSwapPlateComponent: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useAddPlateComponent: vi.fn(() => ({ mutateAsync: vi.fn() })),
  usePlateMacros: vi.fn(() => ({ data: { plates: [] } })),
  useNoPlateActions: vi.fn(() => ({
    markNoPlate: mocks.markNoPlate,
    planMeal: mocks.planMeal,
    editNote: mocks.editNote,
  })),
}))
vi.mock("@/lib/queries/feedback", () => ({
  useRecordFeedback: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useClearFeedback: vi.fn(() => ({ mutateAsync: vi.fn() })),
}))
vi.mock("@/lib/api/plates", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/plates")>()),
  createPlate: vi.fn(),
  addPlateComponent: vi.fn(),
  deletePlate: vi.fn(),
}))
vi.mock("@/lib/toast", () => ({
  toast: Object.assign(vi.fn(), {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
  }),
  toastError: vi.fn(),
}))

import { createPlate } from "@/lib/api/plates"
import { toast } from "@/lib/toast"
import { mockPlate, mockPlateComponent } from "@/test/fixtures"

const today = format(new Date(), "yyyy-MM-dd")
const yesterday = format(subDays(new Date(), 1), "yyyy-MM-dd")
const tomorrow = format(addDays(new Date(), 1), "yyyy-MM-dd")

const mockSlot: TimeSlot = {
  id: 1,
  name_key: "planner.slot_lunch",
  icon: "Utensils",
  sort_order: 0,
  active: true,
}

const days: PlannerDay[] = [
  { date: yesterday, weekday: 0, plates: [] },
  { date: today, weekday: 1, plates: [] },
  { date: tomorrow, weekday: 2, plates: [] },
]

function plate(id: number, date: string): Plate {
  return mockPlate({
    id,
    slot_id: 1,
    date,
    components: [mockPlateComponent({ id: id * 10, plate_id: id, food_id: 1 })],
  })
}

function cell(
  date: string,
  overrides: Partial<AvailabilityCell> = {}
): AvailabilityCell {
  return {
    date,
    slot_id: 1,
    mode: "plan",
    source: "default",
    preset_id: null,
    preset_name: null,
    note: null,
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.dndProps = null
})

describe("PlannerGrid", () => {
  test("renders one header column per day in the days[] prop", async () => {
    renderWithRouter(
      <PlannerGrid
        days={days}
        slots={[mockSlot]}
        rangeFrom={yesterday}
        rangeTo={tomorrow}
      />
    )
    expect(await screen.findByTestId("day-header-0")).toBeInTheDocument()
    expect(await screen.findByTestId("day-header-1")).toBeInTheDocument()
    expect(await screen.findByTestId("day-header-2")).toBeInTheDocument()
  })

  test("marks today's column with data-today", async () => {
    renderWithRouter(
      <PlannerGrid
        days={days}
        slots={[mockSlot]}
        rangeFrom={yesterday}
        rangeTo={tomorrow}
      />
    )
    // slot-cell wrappers carry data-today on per-cell divs; find via slot row
    const container = await screen.findByTestId("slot-row-1")
    const todayCell = container
      .closest(".grid")
      ?.querySelector('[data-today="true"]')
    expect(todayCell).not.toBeNull()
  })

  test("dims past day columns with opacity-60", async () => {
    renderWithRouter(
      <PlannerGrid
        days={days}
        slots={[mockSlot]}
        rangeFrom={yesterday}
        rangeTo={tomorrow}
      />
    )
    const container = await screen.findByTestId("slot-row-1")
    const pastCell = container
      .closest(".grid")
      ?.querySelector('[data-past="true"]')
    expect(pastCell).not.toBeNull()
    expect(pastCell?.className).toContain("opacity-60")
  })
})

describe("PlannerGrid — No plate", () => {
  const noPlateCell = cell(today, {
    mode: "none",
    source: "override",
    note: "Canteen",
  })
  const cellAt = (date: string) => (date === today ? noPlateCell : undefined)

  test("a none cell renders No plate with its note and plans the meal", async () => {
    renderWithRouter(
      <PlannerGrid
        days={days}
        slots={[mockSlot]}
        rangeFrom={yesterday}
        rangeTo={tomorrow}
        cellAt={cellAt}
      />
    )

    const noPlate = await screen.findByText("No plate")
    expect(noPlate).toBeInTheDocument()
    expect(screen.getByText("Canteen")).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole("button", { name: "Plan this meal" })
    )
    expect(mocks.planMeal).toHaveBeenCalledWith(today, 1)
  })

  test("Mark as No plate on an empty cell goes through markNoPlate", async () => {
    renderWithRouter(
      <PlannerGrid
        days={days}
        slots={[mockSlot]}
        rangeFrom={yesterday}
        rangeTo={tomorrow}
        cellAt={cellAt}
      />
    )

    const [first] = await screen.findAllByTestId("slot-empty-no-plate")
    await userEvent.click(first!)
    expect(mocks.markNoPlate).toHaveBeenCalledWith(yesterday, 1, undefined)
  })
})

describe("PlannerGrid — drops", () => {
  const withPlates: PlannerDay[] = [
    { date: yesterday, weekday: 0, plates: [plate(1, yesterday)] },
    { date: today, weekday: 1, plates: [] },
    { date: tomorrow, weekday: 2, plates: [plate(3, tomorrow)] },
  ]

  function dragEnd(
    overData: Record<string, unknown>,
    plateId = 1
  ): DragEndEvent {
    return {
      active: {
        id: `plate:${plateId}`,
        data: {
          current: { plateId, day: 0, slotId: 1, date: yesterday },
        },
      },
      over: { id: "slot:x", data: { current: overData } },
    } as unknown as DragEndEvent
  }

  async function renderGrid(cellAt?: (d: string) => AvailabilityCell) {
    renderWithRouter(
      <PlannerGrid
        days={withPlates}
        slots={[mockSlot]}
        rangeFrom={yesterday}
        rangeTo={tomorrow}
        cellAt={cellAt}
      />
    )
    await screen.findByTestId("slot-row-1")
    expect(mocks.dndProps?.onDragEnd).toBeDefined()
  }

  test("a drop onto a No plate cell is rejected with a toast and no API call", async () => {
    await renderGrid((d) =>
      d === today ? cell(today, { mode: "none" }) : cell(d)
    )

    await mocks.dndProps!.onDragEnd!(
      dragEnd({ day: 1, slotId: 1, date: today, mode: "none" })
    )

    expect(toast.error).toHaveBeenCalledWith("This meal is marked No plate.")
    expect(mocks.updatePlate).not.toHaveBeenCalled()
    expect(createPlate).not.toHaveBeenCalled()
  })

  test("a drop onto an occupied cell is rejected with a toast and no API call", async () => {
    await renderGrid()

    await mocks.dndProps!.onDragEnd!(
      dragEnd({
        day: 2,
        slotId: 1,
        date: tomorrow,
        mode: "plan",
        existingPlateId: 3,
      })
    )

    expect(toast.error).toHaveBeenCalledWith("This meal already has a plate.")
    expect(mocks.updatePlate).not.toHaveBeenCalled()
    expect(createPlate).not.toHaveBeenCalled()
  })

  test("a drop onto an empty plan cell moves the plate", async () => {
    await renderGrid()

    await mocks.dndProps!.onDragEnd!(
      dragEnd({ day: 1, slotId: 1, date: today, mode: "plan" })
    )

    expect(toast.error).not.toHaveBeenCalled()
    expect(mocks.updatePlate).toHaveBeenCalledWith({
      id: 1,
      input: { date: today, slot_id: 1 },
    })
  })
})
