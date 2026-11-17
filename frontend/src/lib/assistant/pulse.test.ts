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

  describe("a change whose ops name their cells", () => {
    const move = mockChange({
      plate_ids: [12],
      session_ids: [],
      dates: ["2026-11-03"],
      params: {
        count: 1,
        ops: [
          {
            op: "move",
            date: "2026-11-03",
            to_date: "2026-11-03",
            pulse: [
              { date: "2026-11-03", slot_id: 3 },
              { date: "2026-11-03", slot_id: 5 },
            ],
          },
        ],
      },
    })

    it("pulses exactly those cells, not every empty cell of the date", () => {
      // The move's source and target cells (the plate itself matches by id).
      expect(cellInChange(cell({ slotId: 3 }), move)).toBe(true)
      expect(cellInChange(cell({ slotId: 5 }), move)).toBe(true)
      // Another empty cell on the same date does not.
      expect(cellInChange(cell({ slotId: 4 }), move)).toBe(false)
      expect(cellInChange(cell({ date: "2026-11-04", slotId: 3 }), move)).toBe(
        false
      )
      // A plate of the change still pulses, wherever it stands.
      expect(
        cellInChange(cell({ slotId: 4, plate: { id: 12 } as never }), move)
      ).toBe(true)
    })

    it("covers away, copy and cook cells and an op's own date and slot", () => {
      const away = mockChange({
        plate_ids: [],
        session_ids: [],
        params: {
          count: 2,
          ops: [
            {
              op: "away",
              pulse: [
                { date: "2026-11-03", slot_id: 3 },
                { date: "2026-11-04", slot_id: 4 },
              ],
            },
            { op: "set_availability", date: "2026-11-05", slot_id: 3 },
          ],
        },
      })
      expect(cellInChange(cell({ slotId: 3 }), away)).toBe(true)
      expect(cellInChange(cell({ date: "2026-11-04", slotId: 4 }), away)).toBe(
        true
      )
      expect(cellInChange(cell({ date: "2026-11-05", slotId: 3 }), away)).toBe(
        true
      )
      expect(cellInChange(cell({ slotId: 4 }), away)).toBe(false)
      expect(cellInChange(cell({ date: "2026-11-05", slotId: 4 }), away)).toBe(
        false
      )
    })
  })

  it("matches nothing without a change", () => {
    expect(cellInChange(cell({}), null)).toBe(false)
  })
})
