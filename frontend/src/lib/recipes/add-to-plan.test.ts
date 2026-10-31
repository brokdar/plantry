import { describe, expect, it } from "vitest"

import { mockPlan, mockPlate } from "@/test/fixtures"

import { openCells } from "./add-to-plan"

describe("openCells", () => {
  it("skips past days, no-plate, routine and occupied cells, in date and meal order", () => {
    const plan = {
      ...mockPlan,
      window: { ...mockPlan.window, today: "2026-11-01" },
      cells: mockPlan.cells.map((c) =>
        c.date === "2026-11-03" && c.slot_id === 1
          ? { ...c, mode: "none" as const }
          : c.date === "2026-11-03" && c.slot_id === 2
            ? { ...c, mode: "routine" as const }
            : c.date === "2026-11-04" && c.slot_id === 2
              ? { ...c, plate: mockPlate() }
              : c
      ),
    }
    const days = openCells([plan])

    // 31 Oct is past; 1 Nov dinner and 2 Nov lunch/dinner hold Chili.
    expect(days[0]).toMatchObject({
      date: "2026-11-01",
      cells: [{ key: "2026-11-01|1" }, { key: "2026-11-01|2" }],
    })
    expect(days[1].cells.map((c) => c.key)).toEqual(["2026-11-02|1"])
    expect(days[2].cells.map((c) => c.key)).toEqual(["2026-11-03|3"])
    expect(days[3].cells.map((c) => c.key)).toEqual([
      "2026-11-04|1",
      "2026-11-04|3",
    ])
    expect(days[0].cells[0].windowStart).toBe("2026-10-31")
  })
})
