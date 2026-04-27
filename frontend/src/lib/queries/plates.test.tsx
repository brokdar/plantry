import { QueryClient } from "@tanstack/react-query"
import { act, renderHook, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"
import { createHookWrapper, renderWithRouter } from "@/test/render"

import type { Plate } from "@/lib/api/plates"

vi.mock("@/lib/api/plates", () => ({
  addPlateComponent: vi.fn(),
  deletePlate: vi.fn(),
  deletePlateComponent: vi.fn(),
  listPlates: vi.fn(),
  updatePlate: vi.fn(),
  updatePlateComponent: vi.fn(),
}))

import { listPlates, updatePlate } from "@/lib/api/plates"

import { plateKeys } from "./keys"
import { usePlatesRange, useUpdatePlate } from "./plates"

function makePlate(overrides?: Partial<Plate>): Plate {
  return {
    id: 1,
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
