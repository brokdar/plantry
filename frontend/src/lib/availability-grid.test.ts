import { describe, expect, it } from "vitest"

import i18n from "@/lib/i18n"
import { mockAvailabilityDefaults } from "@/test/fixtures"

import {
  defaultsToGridValue,
  gridValueToDefaults,
  weekdayColumns,
} from "./availability-grid"

const slots = [{ id: 1 }, { id: 2 }, { id: 3 }]

describe("weekdayColumns", () => {
  it("returns Monday..Sunday keyed 0..6 with short and full labels", () => {
    const cols = weekdayColumns(i18n.getFixedT("en"))
    expect(cols.map((c) => c.key)).toEqual(["0", "1", "2", "3", "4", "5", "6"])
    expect(cols[0]).toEqual({ key: "0", label: "Mon", fullLabel: "Monday" })
    expect(cols[6]).toEqual({ key: "6", label: "Sun", fullLabel: "Sunday" })
  })

  it("localizes the labels", () => {
    const cols = weekdayColumns(i18n.getFixedT("de"))
    expect(cols[0]).toEqual({ key: "0", label: "Mo", fullLabel: "Montag" })
  })
})

describe("defaultsToGridValue", () => {
  it("maps weekday × slot rows to grid keys", () => {
    const value = defaultsToGridValue([
      { weekday: 0, slot_id: 1, mode: "routine", preset_id: 7 },
      { weekday: 4, slot_id: 2, mode: "none", preset_id: null },
      { weekday: 6, slot_id: 3, mode: "plan", preset_id: null },
    ])
    expect(value).toEqual({
      "0:1": { mode: "routine", presetId: 7 },
      "4:2": { mode: "none" },
      "6:3": { mode: "plan" },
    })
  })
})

describe("gridValueToDefaults", () => {
  it("emits the full grid; missing cells are plan", () => {
    const items = gridValueToDefaults({ "2:3": { mode: "none" } }, slots)
    expect(items).toHaveLength(21)
    expect(items[0]).toEqual({
      weekday: 0,
      slot_id: 1,
      mode: "plan",
      preset_id: null,
    })
    expect(items).toContainEqual({
      weekday: 2,
      slot_id: 3,
      mode: "none",
      preset_id: null,
    })
  })

  it("keeps preset_id only for routines", () => {
    const items = gridValueToDefaults(
      {
        "0:1": { mode: "routine", presetId: 7 },
        "0:2": { mode: "none", presetId: 7 },
      },
      slots
    )
    expect(items.slice(0, 2)).toEqual([
      { weekday: 0, slot_id: 1, mode: "routine", preset_id: 7 },
      { weekday: 0, slot_id: 2, mode: "none", preset_id: null },
    ])
  })

  it("round-trips the defaults grid", () => {
    expect(
      gridValueToDefaults(defaultsToGridValue(mockAvailabilityDefaults), slots)
    ).toEqual(mockAvailabilityDefaults)
  })
})
