import { renderHook, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import {
  mockRemovedBatch,
  mockSession,
  mockSessionWithWarnings,
} from "@/test/fixtures"
import { createHookWrapper, invalidatedBy } from "@/test/render"

vi.mock("@/lib/api/sessions", () => ({
  getSession: vi.fn(),
  updateSession: vi.fn(),
  setSessionCooked: vi.fn(),
  getLeftoverCells: vi.fn(),
  addLeftovers: vi.fn(),
  deleteSession: vi.fn(),
}))
vi.mock("@/lib/api/plan", () => ({ restorePlan: vi.fn() }))

const toastMock = vi.hoisted(() => vi.fn())
vi.mock("@/lib/toast", () => ({ toast: toastMock, toastError: vi.fn() }))

import {
  addLeftovers,
  deleteSession,
  getLeftoverCells,
  getSession,
  setSessionCooked,
  updateSession,
} from "@/lib/api/sessions"

import { planKeys, sessionKeys, shoppingKeys, todayKeys } from "./keys"
import {
  useAddLeftovers,
  useDeleteSession,
  useLeftoverCells,
  useSession,
  useSetSessionCooked,
  useUpdateSession,
} from "./sessions"

const SESSION_VIEWS = [
  planKeys.all,
  todayKeys.all,
  shoppingKeys.all,
  sessionKeys.detail(7),
]

beforeEach(() => vi.clearAllMocks())

describe("session queries", () => {
  it("useSession(id)", async () => {
    vi.mocked(getSession).mockResolvedValue(mockSessionWithWarnings)
    const { result } = renderHook(() => useSession(8), {
      wrapper: createHookWrapper(),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(getSession).toHaveBeenCalledWith(8)
    expect(result.current.data?.warnings).toHaveLength(2)
  })

  it("useLeftoverCells(id, count)", async () => {
    const cells = [
      { date: "2026-11-02", slot_id: 2 },
      { date: "2026-11-02", slot_id: 3 },
    ]
    vi.mocked(getLeftoverCells).mockResolvedValue({ cells })
    const { result } = renderHook(() => useLeftoverCells(7, 2), {
      wrapper: createHookWrapper(),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(getLeftoverCells).toHaveBeenCalledWith(7, 2)
    expect(result.current.data?.cells).toEqual(cells)
  })

  it("useLeftoverCells waits for a count", () => {
    renderHook(() => useLeftoverCells(7, 0), { wrapper: createHookWrapper() })
    expect(getLeftoverCells).not.toHaveBeenCalled()
  })
})

// F1-AC2: session mutations invalidate the plan views and the session.
describe("session mutations", () => {
  it("useUpdateSession", async () => {
    vi.mocked(updateSession).mockResolvedValue(mockSession)
    const input = { portions: 8, frozen_portions: 2, note: null }
    expect(
      await invalidatedBy(() => useUpdateSession(), { id: 7, input })
    ).toEqual(SESSION_VIEWS)
    expect(updateSession).toHaveBeenCalledWith(7, input)
  })

  it("useSetSessionCooked", async () => {
    vi.mocked(setSessionCooked).mockResolvedValue(mockSession)
    expect(
      await invalidatedBy(() => useSetSessionCooked(), { id: 7, cooked: true })
    ).toEqual(SESSION_VIEWS)
    expect(setSessionCooked).toHaveBeenCalledWith(7, true)
  })

  it("useAddLeftovers", async () => {
    vi.mocked(addLeftovers).mockResolvedValue(mockSession)
    const cells = [{ date: "2026-11-02", slot_id: 2 }]
    expect(
      await invalidatedBy(() => useAddLeftovers(), { id: 7, cells })
    ).toEqual(SESSION_VIEWS)
    expect(addLeftovers).toHaveBeenCalledWith(7, cells)
  })

  it("useDeleteSession offers Undo", async () => {
    vi.mocked(deleteSession).mockResolvedValue(mockRemovedBatch)
    expect(await invalidatedBy(() => useDeleteSession(), 7)).toEqual(
      SESSION_VIEWS
    )
    expect(deleteSession).toHaveBeenCalledWith(7)
    expect(toastMock).toHaveBeenCalledWith(
      "Batch removed",
      expect.objectContaining({ duration: 6000 })
    )
  })
})
