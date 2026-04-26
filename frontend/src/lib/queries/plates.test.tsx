import { QueryClient } from "@tanstack/react-query"
import { act, renderHook, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"
import { createHookWrapper, renderWithRouter } from "@/test/render"

import type { Week } from "@/lib/api/weeks"
import type { Plate } from "@/lib/api/plates"

vi.mock("@/lib/api/plates", () => ({
  addPlateComponent: vi.fn(),
  deletePlate: vi.fn(),
  deletePlateComponent: vi.fn(),
  listPlates: vi.fn(),
  updatePlate: vi.fn(),
  updatePlateComponent: vi.fn(),
}))

import { listPlates, updatePlate, updatePlateComponent } from "@/lib/api/plates"

import { plateKeys, weekKeys } from "./keys"
import { usePlatesRange, useSwapPlateComponent, useUpdatePlate } from "./plates"

function makePlate(overrides?: Partial<Plate>): Plate {
  return {
    id: 1,
    week_id: 0,
    day: 0,
    slot_id: 1,
    date: "2026-04-26",
    note: null,
    skipped: false,
    components: [],
    created_at: "",
    ...overrides,
  }
}

function makeWeek(): Week {
  return {
    id: 7,
    year: 2026,
    week_number: 16,
    created_at: "",
    plates: [
      {
        id: 100,
        week_id: 7,
        day: 1,
        slot_id: 1,
        date: "2026-04-21",
        note: null,
        skipped: false,
        created_at: "",
        components: [
          {
            id: 200,
            plate_id: 100,
            food_id: 50,
            portions: 1,
            sort_order: 0,
          },
        ],
      },
    ],
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

    qc.setQueryData(weekKeys.byId(7), makeWeek())

    // Seed the range cache so we can verify it gets invalidated.
    const initialPlate = makePlate({ date: "2026-04-21" })
    qc.setQueryData(plateKeys.range("2026-04-21", "2026-04-27"), {
      plates: [initialPlate],
    })

    const updatedPlate = makePlate({ date: "2026-04-22" })
    vi.mocked(updatePlate).mockResolvedValueOnce(updatedPlate)

    const { result } = renderHook(
      () => useUpdatePlate(7, "2026-04-21", "2026-04-27"),
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

  it("does not invalidate range key when no rangeFrom/rangeTo given", async () => {
    const qc = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })

    qc.setQueryData(weekKeys.byId(7), makeWeek())

    // Seed a range cache entry — it must NOT be invalidated.
    qc.setQueryData(plateKeys.range("2026-04-21", "2026-04-27"), {
      plates: [],
    })

    vi.mocked(updatePlate).mockResolvedValueOnce(makePlate())

    const { result } = renderHook(() => useUpdatePlate(7), {
      wrapper: createHookWrapper(qc),
    })

    await act(async () => {
      result.current.mutate({ id: 100, input: { slot_id: 2 } })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const rangeQuery = qc.getQueryState(
      plateKeys.range("2026-04-21", "2026-04-27")
    )
    expect(rangeQuery?.isInvalidated).toBe(false)
  })
})

describe("useSwapPlateComponent", () => {
  beforeEach(() => vi.clearAllMocks())

  it("rolls back optimistic update when API call fails", async () => {
    const qc = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })

    const original = makeWeek()
    qc.setQueryData(weekKeys.byId(7), original)

    vi.mocked(updatePlateComponent).mockRejectedValueOnce(new Error("boom"))

    const { result } = renderHook(() => useSwapPlateComponent(7), {
      wrapper: createHookWrapper(qc),
    })

    await act(async () => {
      result.current.mutate(
        { plateId: 100, pcId: 200, input: { food_id: 999 } },
        { onError: () => {} }
      )
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    const cached = qc.getQueryData<Week>(weekKeys.byId(7))
    expect(cached?.plates[0].components[0].food_id).toBe(50)
  })

  it("optimistically applies swap before resolution", async () => {
    const qc = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })

    qc.setQueryData(weekKeys.byId(7), makeWeek())

    let resolve!: (v: never) => void
    vi.mocked(updatePlateComponent).mockImplementationOnce(
      () => new Promise<never>((r) => (resolve = r))
    )

    const { result } = renderHook(() => useSwapPlateComponent(7), {
      wrapper: createHookWrapper(qc),
    })

    act(() => {
      result.current.mutate({
        plateId: 100,
        pcId: 200,
        input: { food_id: 777 },
      })
    })

    await waitFor(() => {
      const cached = qc.getQueryData<Week>(weekKeys.byId(7))
      expect(cached?.plates[0].components[0].food_id).toBe(777)
    })

    await act(async () => {
      resolve({} as never)
    })
  })
})
