import { QueryClient } from "@tanstack/react-query"
import { act, renderHook, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"
import {
  createHookWrapper,
  invalidatedBy,
  renderWithRouter,
} from "@/test/render"
import {
  mockPlate as makePlate,
  mockPlateComponent,
  mockRemovedBatch,
} from "@/test/fixtures"

vi.mock("@/lib/api/plates", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/plates")>()),
  addPlateComponent: vi.fn(),
  createPlate: vi.fn(),
  deletePlate: vi.fn(),
  deletePlateComponent: vi.fn(),
  listPlates: vi.fn(),
  listPlateMacros: vi.fn(),
  updatePlate: vi.fn(),
  updatePlateComponent: vi.fn(),
}))

import {
  addPlateComponent,
  createPlate,
  deletePlate,
  deletePlateComponent,
  listPlateMacros,
  listPlates,
  updatePlate,
  updatePlateComponent,
} from "@/lib/api/plates"

vi.mock("@/lib/api/plan", () => ({ restorePlan: vi.fn() }))

vi.mock("@/lib/api/availability", () => ({
  getAvailabilityRange: vi.fn(),
  setAvailabilityOverride: vi.fn(),
  clearAvailabilityOverride: vi.fn(),
}))

const toastMock = vi.hoisted(() =>
  Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn(), warning: vi.fn() })
)
vi.mock("@/lib/toast", () => ({ toast: toastMock, toastError: vi.fn() }))

import type { AvailabilityCell } from "@/lib/api/availability"
import {
  clearAvailabilityOverride,
  getAvailabilityRange,
  setAvailabilityOverride,
} from "@/lib/api/availability"

import {
  availabilityKeys,
  planKeys,
  plateKeys,
  shoppingKeys,
  todayKeys,
} from "./keys"
import { nutritionKeys } from "./nutrition"
import {
  __resetPendingPlateDeletesForTests,
  hasPendingPlateDelete,
  registerPendingPlateDelete,
} from "./pending-plate-deletes"
import {
  useAddComponent,
  useAddPlateComponent,
  useCreatePlate,
  useDeletePlate,
  useNoPlateActions,
  usePlannerUpdatePlate,
  usePlateMacros,
  usePlatesRange,
  useRemoveComponent,
  useRemovePlateComponent,
  useSwapPlateComponent,
  useUpdateComponent,
  useUpdatePlate,
  useUpdatePlateComponentQuantity,
} from "./plates"

const PLAN_VIEWS = [planKeys.all, todayKeys.all, shoppingKeys.all]

// F1-AC2: the new plate mutations invalidate exactly the plan views.
describe("plate mutations (§3.1) invalidate the plan views", () => {
  beforeEach(() => vi.clearAllMocks())

  it("useCreatePlate", async () => {
    vi.mocked(createPlate).mockResolvedValueOnce(makePlate())
    const input = {
      date: "2026-11-01",
      slot_id: 3,
      eaters: 2,
      components: [{ food_id: 42 }],
    }
    expect(await invalidatedBy(() => useCreatePlate(), input)).toEqual(
      PLAN_VIEWS
    )
    expect(createPlate).toHaveBeenCalledWith(input)
  })

  it("useUpdatePlate", async () => {
    vi.mocked(updatePlate).mockResolvedValueOnce(makePlate())
    expect(
      await invalidatedBy(() => useUpdatePlate(), {
        id: 1,
        input: { eaters: 3 },
      })
    ).toEqual(PLAN_VIEWS)
    expect(updatePlate).toHaveBeenCalledWith(1, { eaters: 3 })
  })

  it("useDeletePlate also offers Undo", async () => {
    vi.mocked(deletePlate).mockResolvedValueOnce(mockRemovedBatch)
    expect(await invalidatedBy(() => useDeletePlate(), 1)).toEqual(PLAN_VIEWS)
    expect(deletePlate).toHaveBeenCalledWith(1)
    expect(toastMock).toHaveBeenCalledWith(
      "Plate removed",
      expect.objectContaining({ duration: 6000 })
    )
  })

  it("useAddComponent", async () => {
    vi.mocked(addPlateComponent).mockResolvedValueOnce(makePlate())
    const input = { food_id: 42, portions: 1, cook_session_id: 7 }
    expect(
      await invalidatedBy(() => useAddComponent(), { plateId: 2, input })
    ).toEqual(PLAN_VIEWS)
    expect(addPlateComponent).toHaveBeenCalledWith(2, input)
  })

  it("useUpdateComponent", async () => {
    vi.mocked(updatePlateComponent).mockResolvedValueOnce(makePlate())
    expect(
      await invalidatedBy(() => useUpdateComponent(), {
        plateId: 2,
        componentId: 10,
        input: { portions: 1.5 },
      })
    ).toEqual(PLAN_VIEWS)
    expect(updatePlateComponent).toHaveBeenCalledWith(2, 10, { portions: 1.5 })
  })

  it("useRemoveComponent", async () => {
    vi.mocked(deletePlateComponent).mockResolvedValueOnce(makePlate())
    expect(
      await invalidatedBy(() => useRemoveComponent(), {
        plateId: 2,
        componentId: 10,
      })
    ).toEqual(PLAN_VIEWS)
    expect(deletePlateComponent).toHaveBeenCalledWith(2, 10)
  })
})

describe("usePlatesRange", () => {
  beforeEach(() => vi.clearAllMocks())

  it("returns plates from the API for (from, to)", async () => {
    const plate = makePlate({ id: 42, date: "2026-04-28" })
    vi.mocked(listPlates).mockResolvedValueOnce({ plates: [plate] })

    const { result } = renderHook(
      () => usePlatesRange("2026-04-26", "2026-05-02"),
      { wrapper: createHookWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(listPlates).toHaveBeenCalledWith("2026-04-26", "2026-05-02")
    expect(result.current.data?.plates).toHaveLength(1)
    expect(result.current.data?.plates[0].id).toBe(42)
  })

  it("renders plate data via renderWithRouter", async () => {
    const plate = makePlate({ id: 99, date: "2026-04-27" })
    vi.mocked(listPlates).mockResolvedValue({ plates: [plate] })

    function TestComponent() {
      const { data } = usePlatesRange("2026-04-26", "2026-05-02")
      if (!data) return <div>loading</div>
      return <div data-testid="count">{data.plates.length}</div>
    }

    renderWithRouter(<TestComponent />)

    expect(await screen.findByTestId("count")).toHaveTextContent("1")
  })
})

describe("usePlannerUpdatePlate", () => {
  beforeEach(() => vi.clearAllMocks())

  it("invalidates plateKeys.range when rangeFrom/rangeTo are provided", async () => {
    const qc = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })

    // Seed the range cache so we can verify it gets invalidated.
    const initialPlate = makePlate({ date: "2026-04-21" })
    qc.setQueryData(plateKeys.range("2026-04-21", "2026-04-27"), {
      plates: [initialPlate],
    })

    const updatedPlate = makePlate({ date: "2026-04-22" })
    vi.mocked(updatePlate).mockResolvedValueOnce(updatedPlate)

    const { result } = renderHook(
      () => usePlannerUpdatePlate("2026-04-21", "2026-04-27"),
      { wrapper: createHookWrapper(qc) }
    )

    await act(async () => {
      result.current.mutate({ id: 100, input: { date: "2026-04-22" } })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // After onSettled, the range query cache entry must be marked invalid.
    const rangeQuery = qc.getQueryState(
      plateKeys.range("2026-04-21", "2026-04-27")
    )
    expect(rangeQuery?.isInvalidated).toBe(true)
  })

  it("invalidates plateKeys.all when no rangeFrom/rangeTo given", async () => {
    const qc = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })

    // Seed a range cache entry — it must be invalidated via plateKeys.all.
    qc.setQueryData(plateKeys.range("2026-04-21", "2026-04-27"), {
      plates: [],
    })

    vi.mocked(updatePlate).mockResolvedValueOnce(makePlate())

    const { result } = renderHook(() => usePlannerUpdatePlate(), {
      wrapper: createHookWrapper(qc),
    })

    await act(async () => {
      result.current.mutate({ id: 100, input: { slot_id: 2 } })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const rangeQuery = qc.getQueryState(
      plateKeys.range("2026-04-21", "2026-04-27")
    )
    expect(rangeQuery?.isInvalidated).toBe(true)
  })
})

// Day-header kcal in the planner reads from the nutrition range query. If a
// plate mutation refetches plates but leaves nutrition stale, the user sees
// outdated calorie totals (regression: lunch added → day kcal still says
// breakfast-only). Each plate-mutation hook must invalidate nutrition.
describe("plate mutations invalidate nutrition cache", () => {
  beforeEach(() => vi.clearAllMocks())

  function seededClient() {
    const qc = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    qc.setQueryData(nutritionKeys.range("2026-05-04", "2026-05-10"), {
      days: [],
    })
    return qc
  }

  function expectNutritionInvalidated(qc: QueryClient) {
    const state = qc.getQueryState(
      nutritionKeys.range("2026-05-04", "2026-05-10")
    )
    expect(state?.isInvalidated).toBe(true)
  }

  it("usePlannerUpdatePlate", async () => {
    const qc = seededClient()
    vi.mocked(updatePlate).mockResolvedValueOnce(makePlate())

    const { result } = renderHook(
      () => usePlannerUpdatePlate("2026-05-04", "2026-05-10"),
      { wrapper: createHookWrapper(qc) }
    )
    await act(async () => {
      result.current.mutate({ id: 1, input: { slot_id: 2 } })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expectNutritionInvalidated(qc)
  })

  it("useAddPlateComponent", async () => {
    const qc = seededClient()
    vi.mocked(addPlateComponent).mockResolvedValueOnce(makePlate())

    const { result } = renderHook(() => useAddPlateComponent(), {
      wrapper: createHookWrapper(qc),
    })
    await act(async () => {
      result.current.mutate({
        plateId: 1,
        input: { food_id: 1, portions: 1 },
      })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expectNutritionInvalidated(qc)
  })

  it("useSwapPlateComponent", async () => {
    const qc = seededClient()
    vi.mocked(updatePlateComponent).mockResolvedValueOnce(makePlate())

    const { result } = renderHook(() => useSwapPlateComponent(), {
      wrapper: createHookWrapper(qc),
    })
    await act(async () => {
      result.current.mutate({
        plateId: 1,
        pcId: 1,
        input: { food_id: 2 },
      })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expectNutritionInvalidated(qc)
  })

  it("useUpdatePlateComponentQuantity composed", async () => {
    const qc = seededClient()
    vi.mocked(updatePlateComponent).mockResolvedValueOnce(makePlate())

    const { result } = renderHook(() => useUpdatePlateComponentQuantity(), {
      wrapper: createHookWrapper(qc),
    })
    await act(async () => {
      result.current.mutate({
        plateId: 1,
        pcId: 1,
        quantity: { portions: 2 },
      })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expectNutritionInvalidated(qc)
    expect(updatePlateComponent).toHaveBeenCalledWith(1, 1, { portions: 2 })
  })

  it("useUpdatePlateComponentQuantity leaf", async () => {
    const qc = seededClient()
    vi.mocked(updatePlateComponent).mockResolvedValueOnce(makePlate())

    const { result } = renderHook(() => useUpdatePlateComponentQuantity(), {
      wrapper: createHookWrapper(qc),
    })
    await act(async () => {
      result.current.mutate({
        plateId: 1,
        pcId: 2,
        quantity: { amount: 200, unit: "g" },
      })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(updatePlateComponent).toHaveBeenCalledWith(1, 2, {
      amount: 200,
      unit: "g",
    })
  })

  it("useRemovePlateComponent", async () => {
    const qc = seededClient()
    vi.mocked(deletePlateComponent).mockResolvedValueOnce(makePlate())

    const { result } = renderHook(() => useRemovePlateComponent(), {
      wrapper: createHookWrapper(qc),
    })
    await act(async () => {
      result.current.mutate({ plateId: 1, pcId: 1 })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expectNutritionInvalidated(qc)
  })
})

// Regression: a plate "deleted" within the 5 s undo window must not reappear
// when the user immediately fires another plate mutation. The mutation hooks'
// onMutate flushes pending deletes — committing them server-side before the
// new mutation's onSettled refetch runs, so the server no longer returns the
// soft-deleted plate.
describe("plate mutations flush pending deletes via onMutate", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    __resetPendingPlateDeletesForTests()
  })

  it("useAddPlateComponent fires queued delete before adding", async () => {
    const qc = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })

    vi.mocked(deletePlate).mockResolvedValue(mockRemovedBatch)
    vi.mocked(addPlateComponent).mockResolvedValue(makePlate())

    registerPendingPlateDelete(7, makePlate({ id: 7 }), async () => {
      await deletePlate(7)
    })

    const { result } = renderHook(() => useAddPlateComponent(), {
      wrapper: createHookWrapper(qc),
    })

    await act(async () => {
      await result.current.mutateAsync({
        plateId: 1,
        input: { food_id: 1, portions: 1 },
      })
    })

    const deleteOrder = vi.mocked(deletePlate).mock.invocationCallOrder[0]
    const addOrder = vi.mocked(addPlateComponent).mock.invocationCallOrder[0]
    expect(deleteOrder).toBeLessThan(addOrder)
    expect(hasPendingPlateDelete(7)).toBe(false)
  })
})

describe("usePlateMacros", () => {
  beforeEach(() => vi.clearAllMocks())

  it("returns macros from the API for (from, to)", async () => {
    vi.mocked(listPlateMacros).mockResolvedValueOnce({
      plates: [
        {
          plate_id: 42,
          date: "2026-04-28",
          macros: {
            kcal: 620,
            protein: 30,
            fat: 15,
            carbs: 60,
            fiber: 4,
            sodium: 200,
          },
        },
      ],
    })

    const { result } = renderHook(
      () => usePlateMacros("2026-04-26", "2026-05-02"),
      { wrapper: createHookWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(listPlateMacros).toHaveBeenCalledWith("2026-04-26", "2026-05-02")
    expect(result.current.data?.plates[0].plate_id).toBe(42)
    expect(result.current.data?.plates[0].macros.kcal).toBe(620)
  })

  it("invalidates on addComponent because the macros key sits under plateKeys.all", async () => {
    const qc = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    qc.setQueryData(plateKeys.macrosRange("2026-05-04", "2026-05-10"), {
      plates: [],
    })

    vi.mocked(addPlateComponent).mockResolvedValueOnce(makePlate())

    const { result } = renderHook(() => useAddPlateComponent(), {
      wrapper: createHookWrapper(qc),
    })

    await act(async () => {
      result.current.mutate({
        plateId: 1,
        input: { food_id: 1, portions: 1 },
      })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const state = qc.getQueryState(
      plateKeys.macrosRange("2026-05-04", "2026-05-10")
    )
    expect(state?.isInvalidated).toBe(true)
  })
})

describe("useNoPlateActions", () => {
  const from = "2026-10-26"
  const to = "2026-11-01"

  function cell(overrides: Partial<AvailabilityCell> = {}): AvailabilityCell {
    return {
      date: "2026-10-27",
      slot_id: 3,
      mode: "plan",
      source: "default",
      preset_id: null,
      preset_name: null,
      note: null,
      ...overrides,
    }
  }

  function clientWith(c: AvailabilityCell) {
    const qc = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    qc.setQueryData(availabilityKeys.range(from, to), { items: [c] })
    return qc
  }

  const removed = makePlate({
    id: 55,
    date: "2026-10-27",
    slot_id: 3,
    note: "leftovers",
    components: [
      mockPlateComponent({
        id: 2,
        plate_id: 55,
        food_id: 9,
        kind: "leaf",
        portions: undefined,
        cook_session_id: undefined,
        amount: 80,
        unit: "g",
        sort_order: 1,
      }),
      mockPlateComponent({ id: 1, plate_id: 55, food_id: 4, portions: 2 }),
    ],
  })

  beforeEach(() => {
    vi.clearAllMocks()
    __resetPendingPlateDeletesForTests()
  })

  function undoAction(): () => void {
    const opts = toastMock.mock.calls.at(-1)?.[1] as {
      action: { onClick: () => void }
      duration: number
    }
    expect(opts.duration).toBe(6000)
    return opts.action.onClick
  }

  it("markNoPlate puts a none override with remove_plate and offers a 6 s undo", async () => {
    vi.mocked(setAvailabilityOverride).mockResolvedValueOnce({
      cell: cell({ mode: "none", source: "override" }),
      removed_plate: removed,
    })
    const { result } = renderHook(() => useNoPlateActions(from, to), {
      wrapper: createHookWrapper(clientWith(cell())),
    })

    await act(async () => {
      await result.current.markNoPlate("2026-10-27", 3, "Dinner at Mum's")
    })

    expect(setAvailabilityOverride).toHaveBeenCalledWith("2026-10-27", 3, {
      mode: "none",
      note: "Dinner at Mum's",
      remove_plate: true,
    })
    expect(toastMock).toHaveBeenCalledTimes(1)
    undoAction()
  })

  it("undo clears a default-sourced cell, then recreates the plate with its components in order", async () => {
    vi.mocked(setAvailabilityOverride).mockResolvedValueOnce({
      cell: cell({ mode: "none", source: "override" }),
      removed_plate: removed,
    })
    vi.mocked(clearAvailabilityOverride).mockResolvedValueOnce(undefined)
    vi.mocked(createPlate).mockResolvedValueOnce(makePlate({ id: 77 }))
    vi.mocked(addPlateComponent).mockResolvedValue(makePlate())
    const { result } = renderHook(() => useNoPlateActions(from, to), {
      wrapper: createHookWrapper(clientWith(cell())),
    })
    await act(async () => {
      await result.current.markNoPlate("2026-10-27", 3)
    })

    await act(async () => {
      undoAction()()
    })

    await waitFor(() => expect(addPlateComponent).toHaveBeenCalledTimes(2))
    expect(clearAvailabilityOverride).toHaveBeenCalledWith("2026-10-27", 3)
    expect(createPlate).toHaveBeenCalledWith({
      date: "2026-10-27",
      slot_id: 3,
      note: "leftovers",
    })
    const clearOrder = vi.mocked(clearAvailabilityOverride).mock
      .invocationCallOrder[0]!
    const createOrder = vi.mocked(createPlate).mock.invocationCallOrder[0]!
    expect(clearOrder).toBeLessThan(createOrder)
    expect(vi.mocked(addPlateComponent).mock.calls).toEqual([
      [77, { food_id: 4, portions: 2 }],
      [77, { food_id: 9, amount: 80, unit: "g" }],
    ])
  })

  it("undo re-puts a previous override's mode, preset and note", async () => {
    const previous = cell({
      mode: "routine",
      source: "override",
      preset_id: 7,
      preset_name: "Oats",
    })
    vi.mocked(setAvailabilityOverride)
      .mockResolvedValueOnce({ cell: cell({ mode: "none" }) })
      .mockResolvedValueOnce({ cell: previous })
    const { result } = renderHook(() => useNoPlateActions(from, to), {
      wrapper: createHookWrapper(clientWith(previous)),
    })
    await act(async () => {
      await result.current.markNoPlate("2026-10-27", 3)
    })
    await act(async () => {
      undoAction()()
    })

    await waitFor(() =>
      expect(setAvailabilityOverride).toHaveBeenLastCalledWith(
        "2026-10-27",
        3,
        { mode: "routine", preset_id: 7, note: null }
      )
    )
    expect(clearAvailabilityOverride).not.toHaveBeenCalled()
    expect(createPlate).not.toHaveBeenCalled()
  })

  it("planMeal over a plan default only clears the override", async () => {
    vi.mocked(clearAvailabilityOverride).mockResolvedValue(undefined)
    vi.mocked(getAvailabilityRange).mockResolvedValue({
      items: [cell({ mode: "plan", source: "default" })],
    })
    const qc = clientWith(cell({ mode: "none", source: "override" }))
    const invalidate = vi.spyOn(qc, "invalidateQueries")
    const { result } = renderHook(() => useNoPlateActions(from, to), {
      wrapper: createHookWrapper(qc),
    })
    await act(async () => {
      await result.current.planMeal("2026-10-27", 3)
    })
    expect(clearAvailabilityOverride).toHaveBeenCalledWith("2026-10-27", 3)
    expect(getAvailabilityRange).toHaveBeenCalledWith(
      "2026-10-27",
      "2026-10-27"
    )
    expect(setAvailabilityOverride).not.toHaveBeenCalled()
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: availabilityKeys.ranges(),
    })
  })

  it("planMeal over a none default clears the override, then puts plan", async () => {
    vi.mocked(clearAvailabilityOverride).mockResolvedValue(undefined)
    vi.mocked(getAvailabilityRange).mockResolvedValue({
      items: [cell({ mode: "none", source: "default" })],
    })
    vi.mocked(setAvailabilityOverride).mockResolvedValue({ cell: cell() })
    const { result } = renderHook(() => useNoPlateActions(from, to), {
      wrapper: createHookWrapper(
        clientWith(cell({ mode: "none", source: "override" }))
      ),
    })
    await act(async () => {
      await result.current.planMeal("2026-10-27", 3)
    })
    expect(clearAvailabilityOverride).toHaveBeenCalledWith("2026-10-27", 3)
    expect(setAvailabilityOverride).toHaveBeenCalledWith("2026-10-27", 3, {
      mode: "plan",
    })
    const clearOrder = vi.mocked(clearAvailabilityOverride).mock
      .invocationCallOrder[0]!
    const putOrder = vi.mocked(setAvailabilityOverride).mock
      .invocationCallOrder[0]!
    expect(clearOrder).toBeLessThan(putOrder)
  })

  it("planMeal over a none default cell puts plan directly", async () => {
    vi.mocked(setAvailabilityOverride).mockResolvedValue({ cell: cell() })
    const { result } = renderHook(() => useNoPlateActions(from, to), {
      wrapper: createHookWrapper(
        clientWith(cell({ mode: "none", source: "default" }))
      ),
    })
    await act(async () => {
      await result.current.planMeal("2026-10-27", 3)
    })
    expect(setAvailabilityOverride).toHaveBeenCalledWith("2026-10-27", 3, {
      mode: "plan",
    })
    expect(clearAvailabilityOverride).not.toHaveBeenCalled()
    expect(getAvailabilityRange).not.toHaveBeenCalled()
  })

  it("editNote keeps the cell none and writes the note", async () => {
    vi.mocked(setAvailabilityOverride).mockResolvedValue({ cell: cell() })
    const { result } = renderHook(() => useNoPlateActions(from, to), {
      wrapper: createHookWrapper(clientWith(cell({ mode: "none" }))),
    })
    await act(async () => {
      await result.current.editNote("2026-10-27", 3, "Canteen")
    })
    expect(setAvailabilityOverride).toHaveBeenCalledWith("2026-10-27", 3, {
      mode: "none",
      note: "Canteen",
    })
  })
})
