import { describe, expect, it } from "vitest"

import type { BoardCell } from "@/lib/plan/board"
import { mockChange } from "@/test/fixtures"

import { cellInChange } from "./pulse"

function cell(over: Partial<BoardCell>): BoardCell {
  return {
    key: "2026-11-03|3",
    date: "2026-11-03",
    slotId: 3,
    plate: null,
    sessionIds: [],
    cookOf: [],
    ...over,
  } as BoardCell
}

describe("cellInChange", () => {
  it("matches a cell by plate id, or by a session it cooks or eats", () => {
    const change = mockChange({ plate_ids: [12], session_ids: [4] })
    expect(cellInChange(cell({ plate: { id: 12 } as never }), change)).toBe(
      true
    )
    expect(cellInChange(cell({ cookOf: [4] }), change)).toBe(true)
    expect(cellInChange(cell({ sessionIds: [4] }), change)).toBe(true)
    expect(cellInChange(cell({ plate: { id: 13 } as never }), change)).toBe(
      false
    )
  })

  it("falls back to the date for a change without rows or an empty cell", () => {
    const none = mockChange({
      plate_ids: [],
      session_ids: [],
      dates: ["2026-11-03"],
    })
    expect(cellInChange(cell({}), none)).toBe(true)
    expect(cellInChange(cell({ date: "2026-11-04" }), none)).toBe(false)
    const rows = mockChange({
      plate_ids: [12],
      session_ids: [],
      dates: ["2026-11-03"],
    })
    // An empty cell on a touched date: a move's source, an away cell.
    expect(cellInChange(cell({}), rows)).toBe(true)
    expect(cellInChange(cell({ date: "2026-11-04" }), rows)).toBe(false)
    expect(cellInChange(cell({ plate: { id: 13 } as never }), rows)).toBe(false)
  })

  it("matches nothing without a change", () => {
    expect(cellInChange(cell({}), null)).toBe(false)
  })
})
