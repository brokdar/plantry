import i18next from "i18next"
import { describe, expect, it } from "vitest"

import "@/lib/i18n"
import de from "@/lib/i18n/de.json"
import en from "@/lib/i18n/en.json"
import { mockChange } from "@/test/fixtures"

import {
  CHANGE_TEXT_KEYS,
  changeSentence,
  hiddenOpCount,
  statusKey,
} from "./change-text"

type Copy = Record<string, string>

/** A key exists as itself or as a plural form. */
function has(copy: Copy, key: string): boolean {
  return key in copy || `${key}_one` in copy || `${key}_other` in copy
}

describe("change text", () => {
  it("every text_key has en and de copy", () => {
    for (const [lang, copy] of [
      ["en", en.assistant.change],
      ["de", de.assistant.change],
    ] as const) {
      for (const key of CHANGE_TEXT_KEYS) {
        expect(has(copy as Copy, key), `${lang}: ${key}`).toBe(true)
      }
    }
  })

  it("renders the portions line with the frozen part only when frozen", () => {
    const t = i18next.t.bind(i18next)
    const line = (frozen: number) =>
      changeSentence(
        t,
        i18next,
        mockChange({
          text_key: "set_session_portions",
          params: { name: "Chili", portions: 6, frozen },
        })
      )
    expect(line(2)).toBe("Chili: 6 portions, 2 frozen")
    expect(line(0)).toBe("Chili: 6 portions")
  })

  it("composes an apply_changes line from structured ops, capped at three", () => {
    const t = i18next.t.bind(i18next)
    const ops = [
      { op: "add", food: "Gurke", date: "2026-11-03" },
      {
        op: "set_availability",
        slot_key: "slot.dinner",
        mode: "none",
        date: "2026-11-04",
      },
      { op: "set_portions", food: "Chili", portions: 6, date: "2026-11-05" },
      { op: "remove", food: "Reis", date: "2026-11-06" },
    ]
    const line = changeSentence(
      t,
      i18next,
      mockChange({ text_key: "apply_changes", params: { count: 4, ops } })
    )
    expect(line).toBe(
      "4 changes: add Gurke (Tuesday); dinner: no plate (Wednesday); Chili: 6 portions (Thursday); +1 more"
    )
    expect(
      changeSentence(
        t,
        i18next,
        mockChange({
          text_key: "apply_changes",
          params: { count: 1, summary: "Chili 6 portions (2026-11-03)" },
        })
      )
    ).toBe("Chili 6 portions (2026-11-03)")
  })

  it("spells out the meal ops", () => {
    const t = i18next.t.bind(i18next)
    const line = (op: Record<string, unknown>) =>
      changeSentence(
        t,
        i18next,
        mockChange({
          text_key: "apply_changes",
          params: { count: 1, ops: [op] },
        })
      )
    const move = {
      op: "move",
      food: "Chili",
      date: "2026-11-03",
      slot_key: "slot.lunch",
      to_date: "2026-11-04",
      to_slot_key: "slot.dinner",
    }
    expect(line(move)).toBe("Chili to Wednesday dinner")
    expect(line({ ...move, occupant: "swap", other: "Curry" })).toBe(
      "Chili and Curry swapped (Tuesday lunch, Wednesday dinner)"
    )
    expect(line({ ...move, occupant: "drop", other: "Curry" })).toBe(
      "Chili to Wednesday dinner, Curry removed"
    )
    expect(line({ ...move, occupant: "to", other: "Curry" })).toBe(
      "Chili to Wednesday dinner, Curry moved on"
    )
    expect(
      line({ op: "replace", food: "Chili", to: "Dal", date: "2026-11-03" })
    ).toBe("Chili to Dal (Tuesday)")
    expect(
      line({
        op: "away",
        date: "2026-11-04",
        cells: [
          { date: "2026-11-04", slot_key: "slot.lunch" },
          { date: "2026-11-04", slot_key: "slot.dinner" },
        ],
      })
    ).toBe("away: Wednesday lunch, Wednesday dinner")
    expect(
      line({
        op: "move_cook",
        food: "Chili",
        date: "2026-11-03",
        from_date: "2026-11-04",
      })
    ).toBe("Chili now cooked Tuesday")
    expect(
      line({
        op: "copy",
        food: "Chili",
        date: "2026-11-03",
        dates: ["2026-11-04", "2026-11-05"],
        as: "leftover",
      })
    ).toBe("Chili as leftovers on Wednesday, Thursday")
    expect(
      line({ op: "copy", food: "Chili", dates: ["2026-11-04"], as: "fresh" })
    ).toBe("Chili cooked again on Wednesday")
  })

  it("every op key has en and de copy", () => {
    const keys = [
      "op_move",
      "op_move_swap",
      "op_move_drop",
      "op_move_to",
      "op_replace",
      "op_away",
      "op_move_cook",
      "op_copy_leftover",
      "op_copy_fresh",
      "show_less",
    ]
    for (const copy of [en.assistant.change, de.assistant.change]) {
      for (const key of keys) expect(has(copy as Copy, key), key).toBe(true)
    }
  })

  it("lists every op when asked, without the more tail", () => {
    const t = i18next.t.bind(i18next)
    const ops = ["Gurke", "Reis", "Dal", "Brot", "Tee"].map((food) => ({
      op: "add",
      food,
      date: "2026-11-03",
    }))
    const change = mockChange({
      text_key: "apply_changes",
      params: { count: 5, ops },
    })
    expect(hiddenOpCount(change)).toBe(2)
    expect(changeSentence(t, i18next, change, { omitMore: true })).toBe(
      "5 changes: add Gurke (Tuesday); add Reis (Tuesday); add Dal (Tuesday)"
    )
    expect(changeSentence(t, i18next, change, { allOps: true })).toContain(
      "add Tee (Tuesday)"
    )
  })

  it("renders dates as weekdays and slots by name", () => {
    const t = i18next.t.bind(i18next)
    expect(
      changeSentence(
        t,
        i18next,
        mockChange({
          text_key: "set_availability_none",
          params: { date: "2026-11-03", slot: "slot.dinner", mode: "none" },
        })
      )
    ).toBe("Tuesday dinner: no plate")
    expect(
      changeSentence(
        t,
        i18next,
        mockChange({
          text_key: "regenerate_side",
          params: {
            date: "2026-11-03",
            slot: "slot.dinner",
            name: "Roast potatoes",
            count: 1,
          },
        })
      )
    ).toBe("Tuesday dinner: new side Roast potatoes")
  })

  it("falls back to a plain line for an unknown key", () => {
    const t = i18next.t.bind(i18next)
    expect(
      changeSentence(t, i18next, mockChange({ text_key: "nope", params: {} }))
    ).toBe("Your plan changed")
  })

  it("status keys never carry a tool name", () => {
    expect(statusKey("get_plan")).toBe("assistant.status.reading")
    expect(statusKey("regenerate")).toBe("assistant.status.regenerating")
    expect(statusKey("apply_changes")).toBe("assistant.status.updating")
  })
})
