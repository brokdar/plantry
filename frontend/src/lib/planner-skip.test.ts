import { beforeEach, describe, expect, test, vi } from "vitest"

import type { Plate } from "@/lib/api/plates"

import { toggleSkip } from "./planner-skip"

vi.mock("@/lib/api/plates", () => ({
  createPlate: vi.fn(),
}))
vi.mock("@/lib/query-client", () => ({
  queryClient: { invalidateQueries: vi.fn() },
}))

import { createPlate } from "@/lib/api/plates"

function plate(overrides: Partial<Plate> = {}): Plate {
  return {
    id: 10,
    slot_id: 1,
    date: "2026-05-02",
    note: null,
    skipped: false,
    components: [],
    created_at: "2026-05-02T10:00:00Z",
    ...overrides,
  }
}

const baseArgs = {
  date: "2026-05-02",
  slotId: 1,
  rangeFrom: "2026-04-27",
  rangeTo: "2026-05-03",
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe("toggleSkip", () => {
  test("toggles unskipped → skipped, preserves existing note", async () => {
    const setSkipped = vi.fn().mockResolvedValue(undefined)
    await toggleSkip({
      ...baseArgs,
      existing: plate({ skipped: false, note: "carry me" }),
      setSkipped,
    })
    expect(setSkipped).toHaveBeenCalledWith({
      plateId: 10,
      input: { skipped: true, note: "carry me" },
    })
  })

  test("clears note on unskip so re-skip starts blank", async () => {
    const setSkipped = vi.fn().mockResolvedValue(undefined)
    await toggleSkip({
      ...baseArgs,
      existing: plate({ skipped: true, note: "Mom's birthday" }),
      setSkipped,
    })
    expect(setSkipped).toHaveBeenCalledWith({
      plateId: 10,
      input: { skipped: false, note: null },
    })
  })

  test("explicit noteOverride on unskipped slot skips and saves the note", async () => {
    const setSkipped = vi.fn().mockResolvedValue(undefined)
    await toggleSkip({
      ...baseArgs,
      existing: plate({ skipped: false }),
      noteOverride: "Joe's diner",
      setSkipped,
    })
    expect(setSkipped).toHaveBeenCalledWith({
      plateId: 10,
      input: { skipped: true, note: "Joe's diner" },
    })
  })

  test("explicit noteOverride on already-skipped slot only updates the note", async () => {
    const setSkipped = vi.fn().mockResolvedValue(undefined)
    await toggleSkip({
      ...baseArgs,
      existing: plate({ skipped: true, note: "old" }),
      noteOverride: "new",
      setSkipped,
    })
    expect(setSkipped).toHaveBeenCalledWith({
      plateId: 10,
      input: { skipped: true, note: "new" },
    })
  })

  test("noteOverride=null on already-skipped slot clears the note, stays skipped", async () => {
    const setSkipped = vi.fn().mockResolvedValue(undefined)
    await toggleSkip({
      ...baseArgs,
      existing: plate({ skipped: true, note: "old" }),
      noteOverride: null,
      setSkipped,
    })
    expect(setSkipped).toHaveBeenCalledWith({
      plateId: 10,
      input: { skipped: true, note: null },
    })
  })

  test("creates plate first when none exists, then skips", async () => {
    vi.mocked(createPlate).mockResolvedValue(plate({ id: 42 }))
    const setSkipped = vi.fn().mockResolvedValue(undefined)
    await toggleSkip({
      ...baseArgs,
      existing: undefined,
      setSkipped,
    })
    expect(createPlate).toHaveBeenCalledWith({
      date: "2026-05-02",
      slot_id: 1,
    })
    expect(setSkipped).toHaveBeenCalledWith({
      plateId: 42,
      input: { skipped: true, note: null },
    })
  })

  test("creates plate first and stores the noteOverride", async () => {
    vi.mocked(createPlate).mockResolvedValue(plate({ id: 99 }))
    const setSkipped = vi.fn().mockResolvedValue(undefined)
    await toggleSkip({
      ...baseArgs,
      existing: undefined,
      noteOverride: "Eating out",
      setSkipped,
    })
    expect(setSkipped).toHaveBeenCalledWith({
      plateId: 99,
      input: { skipped: true, note: "Eating out" },
    })
  })
})
