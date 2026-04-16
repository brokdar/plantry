import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { Week } from "@/lib/api/weeks"

vi.mock("@/lib/api/plates", () => ({
  addPlateComponent: vi.fn(),
  deletePlate: vi.fn(),
  deletePlateComponent: vi.fn(),
  updatePlate: vi.fn(),
  updatePlateComponent: vi.fn(),
}))

import { updatePlateComponent } from "@/lib/api/plates"

import { weekKeys } from "./keys"
import { useSwapPlateComponent } from "./plates"

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
        note: null,
        created_at: "",
        components: [
          {
            id: 200,
            plate_id: 100,
            component_id: 50,
            portions: 1,
            sort_order: 0,
          },
        ],
      },
    ],
  }
}

function createWrapper(qc: QueryClient) {
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  )
}

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
      wrapper: createWrapper(qc),
    })

    await act(async () => {
      result.current.mutate(
        { plateId: 100, pcId: 200, input: { component_id: 999 } },
        { onError: () => {} }
      )
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    const cached = qc.getQueryData<Week>(weekKeys.byId(7))
    expect(cached?.plates[0].components[0].component_id).toBe(50)
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
      wrapper: createWrapper(qc),
    })

    act(() => {
      result.current.mutate({
        plateId: 100,
        pcId: 200,
        input: { component_id: 777 },
      })
    })

    await waitFor(() => {
      const cached = qc.getQueryData<Week>(weekKeys.byId(7))
      expect(cached?.plates[0].components[0].component_id).toBe(777)
    })

    await act(async () => {
      resolve({} as never)
    })
  })
})
