import { renderHook, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"
import {
  createHookWrapper,
  invalidatedBy,
  renderWithRouter,
} from "@/test/render"
import { mockPlate as makePlate, mockRemovedBatch } from "@/test/fixtures"

vi.mock("@/lib/api/plates", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/plates")>()),
  addPlateComponent: vi.fn(),
  createPlate: vi.fn(),
  deletePlate: vi.fn(),
  deletePlateComponent: vi.fn(),
  listPlates: vi.fn(),
  updatePlate: vi.fn(),
  updatePlateComponent: vi.fn(),
}))

import {
  addPlateComponent,
  createPlate,
  deletePlate,
  deletePlateComponent,
  listPlates,
  updatePlate,
  updatePlateComponent,
} from "@/lib/api/plates"

const toastMock = vi.hoisted(() =>
  Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn(), warning: vi.fn() })
)
vi.mock("@/lib/toast", () => ({ toast: toastMock, toastError: vi.fn() }))

import { planKeys, shoppingKeys, todayKeys } from "./keys"
import {
  useAddComponent,
  useCreatePlate,
  useDeletePlate,
  usePlatesRange,
  useRemoveComponent,
  useUpdateComponent,
  useUpdatePlate,
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
