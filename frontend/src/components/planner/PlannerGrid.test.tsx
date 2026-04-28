import { screen } from "@testing-library/react"
import { addDays, format, subDays } from "date-fns"
import { beforeEach, describe, expect, test, vi } from "vitest"

import { renderWithRouter } from "@/test/render"
import type { TimeSlot } from "@/lib/api/slots"

import { PlannerGrid, type PlannerDay } from "./PlannerGrid"

// Mock all query hooks so PlannerGrid renders without a live server.
vi.mock("@/lib/queries/foods", () => ({
  useFoods: vi.fn(() => ({ data: { items: [] } })),
  useSetFoodFavorite: vi.fn(() => ({ mutateAsync: vi.fn() })),
}))
vi.mock("@/lib/queries/plates", () => ({
  useUpdatePlate: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useDeletePlate: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useSetPlateSkipped: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useSwapPlateComponent: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useAddPlateComponent: vi.fn(() => ({ mutateAsync: vi.fn() })),
}))
vi.mock("@/lib/queries/feedback", () => ({
  useRecordFeedback: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useClearFeedback: vi.fn(() => ({ mutateAsync: vi.fn() })),
}))
vi.mock("@/lib/api/plates", () => ({
  createPlate: vi.fn(),
  addPlateComponent: vi.fn(),
  deletePlate: vi.fn(),
}))

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

beforeEach(() => {
  vi.clearAllMocks()
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
