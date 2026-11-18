import { renderHook, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ApiError } from "@/lib/api/client"
import type { Alternative, Undo } from "@/lib/api/meal"
import { createHookWrapper, invalidatedBy } from "@/test/render"
import { edited, mockPlate, mockUndo } from "@/test/fixtures"

vi.mock("@/lib/api/meal", () => ({
  copyPlate: vi.fn(),
  getAlternatives: vi.fn(),
  moveCook: vi.fn(),
  movePlate: vi.fn(),
  replaceComponent: vi.fn(),
  revertPlan: vi.fn(),
  setAway: vi.fn(),
}))
vi.mock("@/lib/api/plates", () => ({
  addPlateComponent: vi.fn(),
  deletePlateComponent: vi.fn(),
}))
const toastMock = vi.hoisted(() =>
  Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn() })
)
const toastErrorMock = vi.hoisted(() => vi.fn())
vi.mock("@/lib/toast", () => ({ toast: toastMock, toastError: toastErrorMock }))

import {
  copyPlate,
  getAlternatives,
  moveCook,
  movePlate,
  replaceComponent,
  revertPlan,
  setAway,
} from "@/lib/api/meal"
import { addPlateComponent, deletePlateComponent } from "@/lib/api/plates"

import { availabilityKeys, planKeys, shoppingKeys, todayKeys } from "./keys"
import {
  alternativeCalls,
  useAlternatives,
  useApplyAlternative,
  useCopyPlate,
  useMoveCook,
  useMovePlate,
  useSetAway,
} from "./meal"

const PLAN_VIEWS = [planKeys.all, todayKeys.all, shoppingKeys.all]
const result = (undo: Undo = mockUndo) =>
  edited({ plates: [mockPlate()], session_ids: [] }, undo)

function lastToast() {
  const call = toastMock.mock.calls.at(-1)!
  return {
    message: call[0] as string,
    opts: call[1] as { description?: string },
  }
}

beforeEach(() => vi.clearAllMocks())

describe("meal mutations", () => {
  it("useMovePlate invalidates the plan views and offers Undo; a swap says so", async () => {
    vi.mocked(movePlate).mockResolvedValue(result())
    const input = { date: "2026-11-02", slot_id: 3 }
    expect(await invalidatedBy(() => useMovePlate(), { id: 5, input })).toEqual(
      PLAN_VIEWS
    )
    expect(movePlate).toHaveBeenCalledWith(5, input)
    expect(lastToast().message).toBe("Meal moved")

    await invalidatedBy(() => useMovePlate(), {
      id: 5,
      input: { ...input, occupant: "swap" },
    })
    expect(lastToast().message).toBe("Meals swapped")
  })

  it("useMovePlate puts the also-changed words in the toast", async () => {
    vi.mocked(movePlate).mockResolvedValue({
      ...result(),
      also_changed: [
        { session_id: 1, date: "2026-11-02", slot_id: 3, what: "cook_moved" },
      ],
    })
    await invalidatedBy(() => useMovePlate(() => "Lasagne moves"), {
      id: 5,
      input: { date: "2026-11-02", slot_id: 3 },
    })
    expect(lastToast().opts.description).toBe("Lasagne moves")
  })

  it("useMovePlate shows a refusal and offers no Undo", async () => {
    vi.mocked(movePlate).mockRejectedValue(
      new ApiError(409, "error.plate.cell_occupied")
    )
    const { result: r } = renderHook(() => useMovePlate(), {
      wrapper: createHookWrapper(),
    })
    r.current.mutate({ id: 5, input: { date: "2026-11-02", slot_id: 3 } })
    await waitFor(() => expect(toastErrorMock).toHaveBeenCalled())
    expect(toastMock).not.toHaveBeenCalled()
  })

  it("useCopyPlate", async () => {
    vi.mocked(copyPlate).mockResolvedValue(result())
    const cells = [{ date: "2026-11-03", slot_id: 2 }]
    expect(
      await invalidatedBy(() => useCopyPlate(), { id: 5, cells, as: "fresh" })
    ).toEqual(PLAN_VIEWS)
    expect(copyPlate).toHaveBeenCalledWith(5, cells, "fresh")
    expect(lastToast().message).toBe("Meal copied")
  })

  it("useMoveCook", async () => {
    vi.mocked(moveCook).mockResolvedValue(result())
    const cell = { date: "2026-11-01", slot_id: 3 }
    expect(await invalidatedBy(() => useMoveCook(), { id: 7, cell })).toEqual(
      PLAN_VIEWS
    )
    expect(moveCook).toHaveBeenCalledWith(7, cell)
    expect(lastToast().message).toBe("Cooking moved")
  })

  it("useMoveCook shows the backend's refusal", async () => {
    vi.mocked(moveCook).mockRejectedValue(
      new ApiError(400, "error.session.cook_after_meal")
    )
    const { result: r } = renderHook(() => useMoveCook(), {
      wrapper: createHookWrapper(),
    })
    r.current.mutate({ id: 7, cell: { date: "2026-11-09", slot_id: 3 } })
    await waitFor(() => expect(toastErrorMock).toHaveBeenCalled())
    expect(toastMock).not.toHaveBeenCalled()
  })

  it("useSetAway also refreshes availability; a whole day words its Undo", async () => {
    vi.mocked(setAway).mockResolvedValue({
      removed: null,
      also_changed: [],
      undo: mockUndo,
    })
    expect(
      await invalidatedBy(() => useSetAway(), { date: "2026-11-02" })
    ).toEqual([availabilityKeys.all, ...PLAN_VIEWS])
    expect(lastToast().message).toBe("Away all day")

    await invalidatedBy(() => useSetAway(), {
      cells: [{ date: "2026-11-02", slot_id: 3 }],
    })
    expect(lastToast().message).toBe("Marked as No plate")
  })

  it("an empty undo token offers no toast", async () => {
    vi.mocked(copyPlate).mockResolvedValue(
      result({ first_event_id: 0, last_event_id: 0, cells: [] })
    )
    await invalidatedBy(() => useCopyPlate(), {
      id: 5,
      cells: [],
      as: "leftover",
    })
    expect(toastMock).not.toHaveBeenCalled()
  })
})

describe("useAlternatives", () => {
  it("fetches the ranked list for the window and unwraps it", async () => {
    vi.mocked(getAlternatives).mockResolvedValue({
      alternatives: [{ food_id: 1, name: "Dal" } as Alternative],
    })
    const input = { plate_id: 5, what: "meal" as const }
    const { result: r } = renderHook(
      () => useAlternatives("2026-10-31", input),
      { wrapper: createHookWrapper() }
    )
    await waitFor(() => expect(r.current.isSuccess).toBe(true))
    expect(getAlternatives).toHaveBeenCalledWith("2026-10-31", input)
    expect(r.current.data?.alternatives.map((a) => a.name)).toEqual(["Dal"])
  })

  it("waits without an input", () => {
    renderHook(() => useAlternatives("2026-10-31", null), {
      wrapper: createHookWrapper(),
    })
    expect(getAlternatives).not.toHaveBeenCalled()
  })
})

const alt = (ops: Alternative["ops"], extra: Partial<Alternative> = {}) =>
  ({
    food_id: 9,
    name: "Dal",
    source: "fresh",
    plate_kcal: 500,
    plate_protein_g: 20,
    day_kcal: 2000,
    cook_occasions_day: 1,
    ops,
    ...extra,
  }) as Alternative

describe("alternativeCalls", () => {
  beforeEach(() => {
    vi.mocked(replaceComponent).mockResolvedValue(result())
    vi.mocked(addPlateComponent).mockResolvedValue(result() as never)
    vi.mocked(deletePlateComponent).mockResolvedValue(result() as never)
  })

  async function run(plateId: number, a: Alternative) {
    for (const call of alternativeCalls(plateId, a)) await call()
  }

  it("a swap is one replace", async () => {
    await run(
      5,
      alt([{ op: "swap", plate_component_id: 11, food_id: 9, portions: 2 }])
    )
    expect(replaceComponent).toHaveBeenCalledWith(5, {
      plate_component_id: 11,
      food_id: 9,
      portions: 2,
    })
    expect(addPlateComponent).not.toHaveBeenCalled()
    expect(deletePlateComponent).not.toHaveBeenCalled()
  })

  it("a session row is one replace carrying the session", async () => {
    await run(
      5,
      alt(
        [
          {
            op: "replace",
            plate_id: 5,
            plate_component_id: 11,
            food_id: 9,
            session_id: 3,
            portions: 1,
          },
        ],
        { source: "session", session_id: 3 }
      )
    )
    expect(replaceComponent).toHaveBeenCalledTimes(1)
    const input = vi.mocked(replaceComponent).mock.calls[0][1]
    expect(input).toEqual({
      plate_component_id: 11,
      food_id: 9,
      session_id: 3,
      portions: 1,
    })
    expect(addPlateComponent).not.toHaveBeenCalled()
    expect(deletePlateComponent).not.toHaveBeenCalled()
  })

  it("an add alone adds a component, by amount or by portions", async () => {
    await run(5, alt([{ op: "add", food_id: 9, amount: 30, unit: "g" }]))
    expect(addPlateComponent).toHaveBeenLastCalledWith(5, {
      food_id: 9,
      amount: 30,
      unit: "g",
    })
    await run(5, alt([{ op: "add", food_id: 9, session_id: 3, portions: 1 }]))
    expect(addPlateComponent).toHaveBeenLastCalledWith(5, {
      food_id: 9,
      portions: 1,
      cook_session_id: 3,
    })
    expect(replaceComponent).not.toHaveBeenCalled()
  })

  it("a cook_at row is exactly one replace that sends cook_at, no session_id", async () => {
    const cookAt = { date: "2026-11-03", slot_id: 3 }
    const calls = alternativeCalls(
      5,
      alt(
        [
          {
            op: "replace",
            plate_id: 5,
            plate_component_id: 11,
            food_id: 9,
            cook_at: cookAt,
            portions: 2,
          },
        ],
        { source: "cook_at", cook_at: cookAt }
      )
    )
    expect(calls).toHaveLength(1)
    await calls[0]()
    expect(replaceComponent).toHaveBeenCalledTimes(1)
    const input = vi.mocked(replaceComponent).mock.calls[0][1]
    expect(input).toEqual({
      plate_component_id: 11,
      food_id: 9,
      cook_at: cookAt,
      portions: 2,
    })
    expect(input).not.toHaveProperty("session_id")
    expect(addPlateComponent).not.toHaveBeenCalled()
    expect(deletePlateComponent).not.toHaveBeenCalled()
  })

  it("a cook_at add sends cook_at on the component add", async () => {
    const cookAt = { date: "2026-11-03", slot_id: 3 }
    await run(
      5,
      alt(
        [{ op: "add", plate_id: 5, food_id: 9, cook_at: cookAt, portions: 1 }],
        {
          source: "cook_at",
          cook_at: cookAt,
        }
      )
    )
    expect(addPlateComponent).toHaveBeenCalledTimes(1)
    expect(addPlateComponent).toHaveBeenCalledWith(5, {
      food_id: 9,
      portions: 1,
      cook_at: cookAt,
    })
    expect(replaceComponent).not.toHaveBeenCalled()
  })

  it("every other remove deletes its component, after the replace", async () => {
    const calls = alternativeCalls(
      5,
      alt([
        { op: "swap", plate_component_id: 11, food_id: 9 },
        { op: "remove", plate_component_id: 11 },
        { op: "remove", plate_component_id: 12 },
      ])
    )
    expect(calls).toHaveLength(2)
    for (const c of calls) await c()
    expect(deletePlateComponent).toHaveBeenCalledTimes(1)
    expect(deletePlateComponent).toHaveBeenCalledWith(5, 12)
  })
})

describe("useApplyAlternative", () => {
  it("runs the calls in order and offers one Undo over the whole sequence", async () => {
    vi.mocked(replaceComponent).mockResolvedValue(
      result({ first_event_id: 10, last_event_id: 12, cells: [] })
    )
    vi.mocked(deletePlateComponent).mockResolvedValue(
      result({ first_event_id: 13, last_event_id: 14, cells: [] }) as never
    )
    const keys = await invalidatedBy(() => useApplyAlternative(), {
      plateId: 5,
      alt: alt([
        { op: "swap", plate_component_id: 11, food_id: 9 },
        { op: "remove", plate_component_id: 12 },
      ]),
    })
    expect(keys).toEqual(PLAN_VIEWS)
    expect(lastToast().message).toBe("Meal changed")
  })

  it("a failure on the first call shows the error and offers no Undo", async () => {
    vi.mocked(replaceComponent).mockRejectedValue(
      new ApiError(500, "error.internal")
    )
    const { result: r } = renderHook(() => useApplyAlternative(), {
      wrapper: createHookWrapper(),
    })
    r.current.mutate({
      plateId: 5,
      alt: alt([{ op: "swap", plate_component_id: 11, food_id: 9 }]),
    })
    await waitFor(() => expect(toastErrorMock).toHaveBeenCalled())
    expect(toastMock).not.toHaveBeenCalled()
  })

  it("a failure part-way keeps the applied calls undoable and shows the error", async () => {
    vi.mocked(replaceComponent).mockResolvedValue(
      result({ first_event_id: 10, last_event_id: 12, cells: [] })
    )
    vi.mocked(deletePlateComponent).mockRejectedValue(
      new ApiError(500, "error.internal")
    )
    const { result: r } = renderHook(() => useApplyAlternative(), {
      wrapper: createHookWrapper(),
    })
    r.current.mutate({
      plateId: 5,
      alt: alt([
        { op: "swap", plate_component_id: 11, food_id: 9 },
        { op: "remove", plate_component_id: 12 },
      ]),
    })
    await waitFor(() => expect(toastErrorMock).toHaveBeenCalled())
    expect(lastToast().message).toBe("Only part of the change was applied")

    // The Undo reverts the range of the call that did commit.
    const action = (
      toastMock.mock.calls.at(-1)![1] as { action: { onClick: () => void } }
    ).action
    vi.mocked(revertPlan).mockResolvedValue({ plate_ids: [], session_ids: [] })
    action.onClick()
    await waitFor(() =>
      expect(revertPlan).toHaveBeenCalledWith({
        first_event_id: 10,
        last_event_id: 12,
        cells: [],
      })
    )
  })
})
