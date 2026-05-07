import { QueryClient } from "@tanstack/react-query"
import { act, renderHook, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"
import { createHookWrapper, renderWithRouter } from "@/test/render"

import type { Plate } from "@/lib/api/plates"

vi.mock("@/lib/api/plates", () => ({
  addPlateComponent: vi.fn(),
  createPlate: vi.fn(),
  deletePlate: vi.fn(),
  deletePlateComponent: vi.fn(),
  listPlates: vi.fn(),
  setPlateSkipped: vi.fn(),
  updatePlate: vi.fn(),
  updatePlateComponent: vi.fn(),
}))

import {
  addPlateComponent,
  createPlate,
  deletePlate,
  deletePlateComponent,
  listPlates,
  setPlateSkipped,
  updatePlate,
  updatePlateComponent,
} from "@/lib/api/plates"

import { plateKeys } from "./keys"
import { nutritionKeys } from "./nutrition"
import {
  __resetPendingPlateDeletesForTests,
  hasPendingPlateDelete,
  registerPendingPlateDelete,
} from "./pending-plate-deletes"
import {
  useAddPlateComponent,
  useCreatePlate,
  useDeletePlate,
  usePlatesRange,
  useRemovePlateComponent,
  useSetPlateSkipped,
  useSwapPlateComponent,
  useUpdatePlate,
  useUpdatePlateComponentPortions,
} from "./plates"

function makePlate(overrides?: Partial<Plate>): Plate {
  return {
    id: 1,
    slot_id: 1,
    date: "2026-04-26",
    note: null,
    skipped: false,
    components: [],
    created_at: "",
    ...overrides,
  }
}

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

describe("useUpdatePlate", () => {
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
      () => useUpdatePlate("2026-04-21", "2026-04-27"),
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

    const { result } = renderHook(() => useUpdatePlate(), {
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

  it("useCreatePlate", async () => {
    const qc = seededClient()
    vi.mocked(createPlate).mockResolvedValueOnce(makePlate())

    const { result } = renderHook(
      () => useCreatePlate("2026-05-04", "2026-05-10"),
      { wrapper: createHookWrapper(qc) }
    )
    await act(async () => {
      result.current.mutate({ date: "2026-05-04", slot_id: 1 })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expectNutritionInvalidated(qc)
  })

  it("useUpdatePlate", async () => {
    const qc = seededClient()
    vi.mocked(updatePlate).mockResolvedValueOnce(makePlate())

    const { result } = renderHook(
      () => useUpdatePlate("2026-05-04", "2026-05-10"),
      { wrapper: createHookWrapper(qc) }
    )
    await act(async () => {
      result.current.mutate({ id: 1, input: { slot_id: 2 } })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expectNutritionInvalidated(qc)
  })

  it("useDeletePlate", async () => {
    const qc = seededClient()
    vi.mocked(deletePlate).mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useDeletePlate(), {
      wrapper: createHookWrapper(qc),
    })
    await act(async () => {
      result.current.mutate(1)
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expectNutritionInvalidated(qc)
  })

  it("useAddPlateComponent", async () => {
    const qc = seededClient()
    vi.mocked(addPlateComponent).mockResolvedValueOnce({
      id: 1,
      plate_id: 1,
      food_id: 1,
      portions: 1,
      sort_order: 0,
    })

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
    vi.mocked(updatePlateComponent).mockResolvedValueOnce({
      id: 1,
      plate_id: 1,
      food_id: 2,
      portions: 1,
      sort_order: 0,
    })

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

  it("useUpdatePlateComponentPortions", async () => {
    const qc = seededClient()
    vi.mocked(updatePlateComponent).mockResolvedValueOnce({
      id: 1,
      plate_id: 1,
      food_id: 1,
      portions: 2,
      sort_order: 0,
    })

    const { result } = renderHook(() => useUpdatePlateComponentPortions(), {
      wrapper: createHookWrapper(qc),
    })
    await act(async () => {
      result.current.mutate({ plateId: 1, pcId: 1, portions: 2 })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expectNutritionInvalidated(qc)
  })

  it("useSetPlateSkipped", async () => {
    const qc = seededClient()
    vi.mocked(setPlateSkipped).mockResolvedValueOnce(
      makePlate({ skipped: true })
    )

    const { result } = renderHook(() => useSetPlateSkipped(), {
      wrapper: createHookWrapper(qc),
    })
    await act(async () => {
      result.current.mutate({ plateId: 1, input: { skipped: true } })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expectNutritionInvalidated(qc)
  })

  it("useRemovePlateComponent", async () => {
    const qc = seededClient()
    vi.mocked(deletePlateComponent).mockResolvedValueOnce(undefined)

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

  it("useCreatePlate fires queued deletePlate before createPlate", async () => {
    const qc = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })

    vi.mocked(deletePlate).mockResolvedValue(undefined)
    vi.mocked(createPlate).mockResolvedValue(makePlate({ id: 99 }))

    registerPendingPlateDelete(7, makePlate({ id: 7 }), () => deletePlate(7))
    expect(hasPendingPlateDelete(7)).toBe(true)

    const { result } = renderHook(
      () => useCreatePlate("2026-05-04", "2026-05-10"),
      { wrapper: createHookWrapper(qc) }
    )

    await act(async () => {
      await result.current.mutateAsync({ date: "2026-05-04", slot_id: 1 })
    })

    expect(deletePlate).toHaveBeenCalledWith(7)
    expect(createPlate).toHaveBeenCalledTimes(1)
    const deleteOrder = vi.mocked(deletePlate).mock.invocationCallOrder[0]
    const createOrder = vi.mocked(createPlate).mock.invocationCallOrder[0]
    expect(deleteOrder).toBeLessThan(createOrder)
    expect(hasPendingPlateDelete(7)).toBe(false)
  })

  it("useDeletePlate flushes other pending deletes before its own delete", async () => {
    const qc = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })

    vi.mocked(deletePlate).mockResolvedValue(undefined)

    registerPendingPlateDelete(7, makePlate({ id: 7 }), () => deletePlate(7))

    const { result } = renderHook(() => useDeletePlate(), {
      wrapper: createHookWrapper(qc),
    })

    await act(async () => {
      await result.current.mutateAsync(99)
    })

    const calls = vi.mocked(deletePlate).mock.calls.map((c) => c[0])
    expect(calls).toEqual([7, 99])
    expect(hasPendingPlateDelete(7)).toBe(false)
  })

  it("useAddPlateComponent fires queued delete before adding", async () => {
    const qc = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })

    vi.mocked(deletePlate).mockResolvedValue(undefined)
    vi.mocked(addPlateComponent).mockResolvedValue({
      id: 1,
      plate_id: 1,
      food_id: 1,
      portions: 1,
      sort_order: 0,
    })

    registerPendingPlateDelete(7, makePlate({ id: 7 }), () => deletePlate(7))

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
