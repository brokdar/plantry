import { afterEach, beforeEach, describe, expect, test, vi } from "vitest"

import type { Plate } from "@/lib/api/plates"

import {
  PENDING_DELETE_DELAY_MS,
  __resetPendingPlateDeletesForTests,
  cancelPendingPlateDelete,
  flushPendingPlateDeletes,
  hasPendingPlateDelete,
  registerPendingPlateDelete,
} from "./pending-plate-deletes"

function plate(id: number): Plate {
  return {
    id,
    slot_id: 1,
    date: "2026-05-05",
    note: null,
    skipped: false,
    components: [],
    created_at: "",
  }
}

beforeEach(() => {
  vi.useFakeTimers()
  __resetPendingPlateDeletesForTests()
})

afterEach(() => {
  vi.useRealTimers()
})

describe("pending-plate-deletes", () => {
  test("registers and reports pending state", () => {
    const fire = vi.fn().mockResolvedValue(undefined)
    expect(hasPendingPlateDelete(1)).toBe(false)
    registerPendingPlateDelete(1, plate(1), fire)
    expect(hasPendingPlateDelete(1)).toBe(true)
  })

  test("cancel clears the timer and returns the snapshot", async () => {
    const fire = vi.fn().mockResolvedValue(undefined)
    const snap = plate(1)
    registerPendingPlateDelete(1, snap, fire)
    expect(cancelPendingPlateDelete(1)).toBe(snap)
    expect(hasPendingPlateDelete(1)).toBe(false)
    await vi.advanceTimersByTimeAsync(PENDING_DELETE_DELAY_MS + 100)
    expect(fire).not.toHaveBeenCalled()
  })

  test("fires after delay if not cancelled", async () => {
    const fire = vi.fn().mockResolvedValue(undefined)
    registerPendingPlateDelete(1, plate(1), fire)
    await vi.advanceTimersByTimeAsync(PENDING_DELETE_DELAY_MS + 1)
    expect(fire).toHaveBeenCalledTimes(1)
    expect(hasPendingPlateDelete(1)).toBe(false)
  })

  test("flushAll fires every pending entry and awaits completion", async () => {
    let resolve1: () => void = () => {}
    let resolve2: () => void = () => {}
    const fire1 = vi.fn(
      () =>
        new Promise<void>((r) => {
          resolve1 = r
        })
    )
    const fire2 = vi.fn(
      () =>
        new Promise<void>((r) => {
          resolve2 = r
        })
    )

    registerPendingPlateDelete(1, plate(1), fire1)
    registerPendingPlateDelete(2, plate(2), fire2)

    const flushed = flushPendingPlateDeletes()
    // Flush picks up both entries synchronously before yielding.
    expect(fire1).toHaveBeenCalledTimes(1)
    expect(fire2).toHaveBeenCalledTimes(1)

    resolve1()
    resolve2()
    await flushed

    expect(hasPendingPlateDelete(1)).toBe(false)
    expect(hasPendingPlateDelete(2)).toBe(false)
  })

  test("flushAll on empty registry resolves immediately", async () => {
    await expect(flushPendingPlateDeletes()).resolves.toBeUndefined()
  })

  test("re-registering same id replaces the previous entry", async () => {
    const fire1 = vi.fn().mockResolvedValue(undefined)
    const fire2 = vi.fn().mockResolvedValue(undefined)
    registerPendingPlateDelete(1, plate(1), fire1)
    registerPendingPlateDelete(1, plate(1), fire2)
    await vi.advanceTimersByTimeAsync(PENDING_DELETE_DELAY_MS + 1)
    expect(fire1).not.toHaveBeenCalled()
    expect(fire2).toHaveBeenCalledTimes(1)
  })

  test("flushAll while a fire is in-flight does not double-fire", async () => {
    let resolve1: () => void = () => {}
    const fire1 = vi.fn(
      () =>
        new Promise<void>((r) => {
          resolve1 = r
        })
    )
    registerPendingPlateDelete(1, plate(1), fire1)

    const first = flushPendingPlateDeletes()
    const second = flushPendingPlateDeletes()

    expect(fire1).toHaveBeenCalledTimes(1)
    resolve1()
    await Promise.all([first, second])
    expect(fire1).toHaveBeenCalledTimes(1)
  })
})
