import { describe, expect, it } from "vitest"

import { mockPlan } from "@/test/fixtures"

import { replanFrom } from "./replan"

const win = mockPlan.window // 31 Oct – 6 Nov

describe("replanFrom", () => {
  it.each([
    ["today inside the window", "2026-11-03", "2026-11-03"],
    ["today on the window's last day", "2026-11-06", "2026-11-06"],
    ["a window ahead starts at its start", "2026-10-20", "2026-10-31"],
    ["a past window has none", "2026-11-07", null],
  ])("%s", (_, today, want) => {
    expect(replanFrom({ ...win, today })).toBe(want)
  })
})
