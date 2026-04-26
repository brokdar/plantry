import { describe, expect, test } from "vitest"

import type { Week } from "@/lib/api/weeks"

import {
  findPlateAt,
  findPlateForComponent,
  patchAddComponent,
  patchAddPlate,
  patchDeletePlate,
  patchRemoveComponent,
  patchSwapComponent,
  patchUpdateComponentPortions,
  patchUpdatePlate,
} from "./plate-patches"

function week(): Week {
  return {
    id: 1,
    year: 2026,
    week_number: 16,
    created_at: "",
    plates: [
      {
        id: 10,
        week_id: 1,
        day: 0,
        slot_id: 5,
        date: "2026-04-14",
        note: null,
        skipped: false,
        created_at: "",
        components: [
          {
            id: 100,
            plate_id: 10,
            food_id: 1000,
            portions: 1,
            sort_order: 0,
          },
          {
            id: 101,
            plate_id: 10,
            food_id: 1001,
            portions: 2,
            sort_order: 1,
          },
        ],
      },
      {
        id: 11,
        week_id: 1,
        day: 1,
        slot_id: 5,
        date: "2026-04-15",
        note: "leftovers",
        skipped: false,
        created_at: "",
        components: [
          {
            id: 102,
            plate_id: 11,
            food_id: 1002,
            portions: 1,
            sort_order: 0,
          },
        ],
      },
    ],
  }
}

describe("plate patches", () => {
  test("patchAddPlate appends a plate", () => {
    const w = week()
    const next = patchAddPlate(w, {
      id: 12,
      week_id: 1,
      day: 2,
      slot_id: 5,
      date: "2026-04-16",
      note: null,
      skipped: false,
      created_at: "",
      components: [],
    })
    expect(next?.plates).toHaveLength(3)
    expect(w.plates).toHaveLength(2) // immutable
  })

  test("patchDeletePlate removes by id", () => {
    const next = patchDeletePlate(week(), 10)
    expect(next?.plates.map((p) => p.id)).toEqual([11])
  })

  test("patchUpdatePlate merges fields", () => {
    const next = patchUpdatePlate(week(), 10, { day: 4, note: "n" })
    const p = next?.plates.find((p) => p.id === 10)
    expect(p?.day).toBe(4)
    expect(p?.note).toBe("n")
    expect(p?.slot_id).toBe(5)
  })

  test("patchAddComponent appends to plate.components", () => {
    const next = patchAddComponent(week(), 10, {
      id: 999,
      plate_id: 10,
      food_id: 2000,
      portions: 1,
      sort_order: 99,
    })
    const p = next?.plates.find((p) => p.id === 10)
    expect(p?.components).toHaveLength(3)
  })

  test("patchSwapComponent replaces food_id, default keeps portions", () => {
    const next = patchSwapComponent(week(), 100, 9999)
    const pc = next?.plates[0].components[0]
    expect(pc?.food_id).toBe(9999)
    expect(pc?.portions).toBe(1)
  })

  test("patchSwapComponent applies portionsOverride", () => {
    const next = patchSwapComponent(week(), 101, 9999, 5)
    const pc = next?.plates[0].components[1]
    expect(pc?.food_id).toBe(9999)
    expect(pc?.portions).toBe(5)
  })

  test("patchUpdateComponentPortions sets portions only", () => {
    const next = patchUpdateComponentPortions(week(), 100, 7)
    expect(next?.plates[0].components[0].portions).toBe(7)
  })

  test("patchRemoveComponent strips by pcId", () => {
    const next = patchRemoveComponent(week(), 100)
    expect(next?.plates[0].components.map((pc) => pc.id)).toEqual([101])
  })

  test("findPlateAt locates by day+slot", () => {
    const w = week()
    expect(findPlateAt(w, 0, 5)?.id).toBe(10)
    expect(findPlateAt(w, 9, 5)).toBeUndefined()
  })

  test("findPlateForComponent walks plates", () => {
    const w = week()
    expect(findPlateForComponent(w, 102)?.id).toBe(11)
  })

  test("all patches are no-ops on undefined week", () => {
    expect(patchAddPlate(undefined, {} as never)).toBeUndefined()
    expect(patchDeletePlate(undefined, 1)).toBeUndefined()
    expect(patchAddComponent(undefined, 1, {} as never)).toBeUndefined()
    expect(patchSwapComponent(undefined, 1, 2)).toBeUndefined()
    expect(patchRemoveComponent(undefined, 1)).toBeUndefined()
    expect(patchUpdateComponentPortions(undefined, 1, 1)).toBeUndefined()
    expect(patchUpdatePlate(undefined, 1, {})).toBeUndefined()
  })
})
