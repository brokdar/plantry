import { describe, expect, it } from "vitest"

import type { Plan } from "@/lib/api/plan"
import type { Session } from "@/lib/api/sessions"
import i18n from "@/lib/i18n"
import {
  mockPlan,
  mockPlate,
  mockPlateComponent,
  mockSession,
} from "@/test/fixtures"

import {
  alsoChangedLines,
  cellLabel,
  cookCells,
  emptyReasonText,
  lighterKcal,
  targetCells,
} from "./meal-actions"

const t = i18n.t.bind(i18n)
const slotName = (id: number) => `slot${id}`

const slots = [1, 2, 3].map((id) => ({
  id,
  name_key: `slot.${id}`,
  icon: "",
  sort_order: id,
}))

function cell(
  date: string,
  slot_id: number,
  mode: "plan" | "none" | "routine" = "plan",
  plate: ReturnType<typeof mockPlate> | null = null
) {
  return { date, slot_id, mode, plate } as unknown as Plan["cells"][number]
}

const plan: Plan = {
  ...mockPlan,
  slots,
  cells: [
    cell(
      "2026-11-02",
      2,
      "plan",
      mockPlate({ id: 2, components: [mockPlateComponent()] })
    ),
    cell("2026-11-01", 3),
    cell("2026-11-01", 1),
    cell("2026-11-01", 2, "none"),
    cell("2026-10-31", 1),
    cell("2026-11-03", 1, "routine"),
  ],
}

describe("targetCells", () => {
  it("lists the free plan cells from today on, in board order", () => {
    expect(targetCells(plan, "2026-11-01").map((c) => c.key)).toEqual([
      "2026-11-01|1",
      "2026-11-01|3",
    ])
  })

  it("keeps the taken cells with their plate when asked, and drops `except`", () => {
    const keys = targetCells(plan, "2026-11-01", {
      taken: true,
      except: "2026-11-01|1",
    })
    expect(keys.map((c) => c.key)).toEqual(["2026-11-01|3", "2026-11-02|2"])
    expect(keys[1].plateId).toBe(2)
    expect(keys[1].name).toBe("Chicken Curry")
  })
})

describe("cookCells", () => {
  const session: Session = {
    ...mockSession,
    cook_date: "2026-11-02",
    cook_slot_id: 2,
    keep_days: 3,
    links: [
      {
        ...mockSession.links[0],
        date: "2026-11-02",
        slot_id: 2,
        kind: "fresh",
      },
      {
        ...mockSession.links[1],
        date: "2026-11-04",
        slot_id: 1,
        kind: "leftover",
      },
    ],
  }
  const wide: Plan = {
    ...plan,
    cells: [
      cell("2026-10-31", 1),
      cell("2026-11-01", 1),
      cell("2026-11-02", 2, "plan", mockPlate({ id: 1 })),
      cell("2026-11-02", 3),
      cell("2026-11-04", 1, "plan", mockPlate({ id: 2 })),
    ],
  }

  it("keeps cells on or before the first meal and within keep days of the last", () => {
    expect(cookCells(wide, session, "2026-11-01").map((c) => c.key)).toEqual([
      "2026-11-01|1",
    ])
  })

  it("ignores frozen links when finding the meals", () => {
    const frozen: Session = {
      ...session,
      links: [
        session.links[0],
        { ...session.links[1], kind: "frozen", date: "2026-12-30" },
      ],
    }
    expect(cookCells(wide, frozen, "2026-10-31").map((c) => c.key)).toEqual([
      "2026-10-31|1",
      "2026-11-01|1",
    ])
  })
})

describe("lighterKcal", () => {
  it("spreads the day's target over its plated meals", () => {
    const p: Plan = {
      ...plan,
      days: [{ ...mockPlan.days[0], date: "2026-11-02", target_kcal: 2000 }],
      cells: [
        cell("2026-11-02", 1, "plan", mockPlate({ id: 1 })),
        cell("2026-11-02", 2, "plan", mockPlate({ id: 2 })),
      ],
    }
    expect(lighterKcal(p, "2026-11-02")).toBe(1000)
  })

  it("falls back to the plan's target and is undefined without any", () => {
    expect(lighterKcal({ ...plan, days: [] }, "2026-11-02")).toBe(2200)
    expect(
      lighterKcal({ ...plan, days: [], targets: null }, "2026-11-02")
    ).toBeUndefined()
  })
})

describe("words", () => {
  it("cellLabel names weekday and slot", () => {
    expect(
      cellLabel({ date: "2026-11-05", slot_id: 3 }, t, i18n, slotName)
    ).toBe("Thu slot3")
  })

  it("alsoChangedLines says each change in plain words", () => {
    const base = { session_id: mockSession.id, date: "2026-11-05", slot_id: 3 }
    expect(
      alsoChangedLines(
        [
          { ...base, what: "cook_moved" },
          { ...base, what: "cooks_fresh" },
          { ...base, what: "batch_resized" },
        ],
        plan,
        t,
        i18n,
        slotName
      )
    ).toEqual([
      "Chili will now be cooked Thu slot3",
      "Thu slot3 will cook Chili fresh",
      "The Chili batch changes size",
    ])
  })
})

describe("emptyReasonText", () => {
  it("has copy for nutrition_missing in en and de", async () => {
    const reason = {
      candidates: 3,
      dropped: { "H8.variety_repeat": 1, nutrition_missing: 2 },
    }
    await i18n.changeLanguage("en")
    expect(emptyReasonText(reason, i18n.t.bind(i18n))).toBe(
      "Nothing fits. Of 3 matching dishes: would have no nutrition data (2), eaten recently (1)."
    )
    await i18n.changeLanguage("de")
    expect(emptyReasonText(reason, i18n.t.bind(i18n))).toContain(
      "hätte keine Nährwertdaten (2)"
    )
    await i18n.changeLanguage("en")
  })
})
