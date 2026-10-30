import { describe, expect, it } from "vitest"

import { mockPlan, mockSession } from "@/test/fixtures"
import type { Session } from "@/lib/api/sessions"

import { batchColour, colourBatches } from "./batch-colours"

function session(id: number, cook_date: string, cook_slot_id: number): Session {
  return { ...mockSession, id, cook_date, cook_slot_id }
}

describe("batchColour", () => {
  it("cycles 1–6 then starts again at 1", () => {
    expect([0, 1, 2, 3, 4, 5, 6, 7].map(batchColour)).toEqual([
      1, 2, 3, 4, 5, 6, 1, 2,
    ])
  })
})

describe("colourBatches (F2-AC1)", () => {
  it("assigns colours 1–6 then 1 again in cook-cell order", () => {
    // Shuffled input: the order must come from (date, slot sort_order).
    const sessions = [
      session(10, "2026-11-03", 2),
      session(11, "2026-10-31", 3),
      session(12, "2026-11-01", 3),
      session(13, "2026-11-05", 3),
      session(14, "2026-11-01", 2),
      session(15, "2026-11-04", 1),
      session(16, "2026-11-06", 2),
    ]
    const batches = colourBatches(sessions, mockPlan.slots)
    expect(batches.map((b) => [b.session.id, b.colour])).toEqual([
      [11, 1],
      [14, 2],
      [12, 3],
      [10, 4],
      [15, 5],
      [13, 6],
      [16, 1],
    ])
  })

  it("orders by slot sort_order, not slot id, and breaks ties by session id", () => {
    const slots = [
      { id: 1, sort_order: 2 },
      { id: 2, sort_order: 0 },
    ]
    const batches = colourBatches(
      [
        session(5, "2026-11-01", 1),
        session(4, "2026-11-01", 2),
        session(3, "2026-11-01", 1),
      ],
      slots
    )
    expect(batches.map((b) => b.session.id)).toEqual([4, 3, 5])
  })

  it("does not mutate its input", () => {
    const sessions = [session(2, "2026-11-02", 1), session(1, "2026-11-01", 1)]
    colourBatches(sessions, mockPlan.slots)
    expect(sessions.map((s) => s.id)).toEqual([2, 1])
  })
})
