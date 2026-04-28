import { describe, expect, it } from "vitest"
import { computeAnchor, windowRange } from "./planner-window"

// Helper: build a Date at midnight local time for a given YYYY-MM-DD string.
function d(ymd: string): Date {
  const [y, m, day] = ymd.split("-").map(Number)
  return new Date(y, m - 1, day)
}

// 2026-04-27 is a Monday (JS getDay() === 1).
// Day-of-week reference:
//   Mon 2026-04-27  JS=1  backend=0
//   Tue 2026-04-28  JS=2  backend=1
//   Wed 2026-04-29  JS=3  backend=2
//   Thu 2026-04-30  JS=4  backend=3
//   Fri 2026-05-01  JS=5  backend=4
//   Sat 2026-05-02  JS=6  backend=5
//   Sun 2026-05-03  JS=0  backend=6

const MONDAY = d("2026-04-27")

describe("computeAnchor", () => {
  describe('mode "today"', () => {
    it("returns injected today, normalised to midnight", () => {
      const injected = new Date(2026, 3, 27, 14, 30, 0) // 14:30 local
      const result = computeAnchor({
        mode: "today",
        shoppingDay: 5, // Saturday (backend)
        weekStartsOn: "monday",
        today: injected,
      })
      expect(result).toEqual(d("2026-04-27"))
    })
  })

  describe('mode "next_shopping_day"', () => {
    it("returns today when today IS the shopping day", () => {
      // today = Monday (backend shoppingDay = 0 = Monday)
      const result = computeAnchor({
        mode: "next_shopping_day",
        shoppingDay: 0, // Monday
        weekStartsOn: "monday",
        today: MONDAY,
      })
      expect(result).toEqual(d("2026-04-27"))
    })

    it("returns the next future occurrence when today is NOT the shopping day", () => {
      // today = Monday, shoppingDay = Saturday (backend=5, JS=6)
      // Next Saturday from 2026-04-27 is 2026-05-02
      const result = computeAnchor({
        mode: "next_shopping_day",
        shoppingDay: 5, // Saturday (backend)
        weekStartsOn: "monday",
        today: MONDAY,
      })
      expect(result).toEqual(d("2026-05-02"))
    })

    it("returns the next Sunday when today is Monday and shoppingDay is Sunday (backend=6)", () => {
      // Next Sunday from 2026-04-27 is 2026-05-03
      const result = computeAnchor({
        mode: "next_shopping_day",
        shoppingDay: 6, // Sunday (backend)
        weekStartsOn: "monday",
        today: MONDAY,
      })
      expect(result).toEqual(d("2026-05-03"))
    })
  })

  describe('mode "fixed_weekday"', () => {
    it("returns today when today IS the fixed weekday", () => {
      // today = Monday, fixedWeekday = 0 (Monday, backend)
      const result = computeAnchor({
        mode: "fixed_weekday",
        shoppingDay: 5,
        fixedWeekday: 0, // Monday (backend)
        weekStartsOn: "monday",
        today: MONDAY,
      })
      expect(result).toEqual(d("2026-04-27"))
    })

    it("returns the next occurrence when today is NOT the fixed weekday", () => {
      // today = Monday, fixedWeekday = Wednesday (backend=2, JS=3)
      // Next Wednesday from 2026-04-27 is 2026-04-29
      const result = computeAnchor({
        mode: "fixed_weekday",
        shoppingDay: 5,
        fixedWeekday: 2, // Wednesday (backend)
        weekStartsOn: "monday",
        today: MONDAY,
      })
      expect(result).toEqual(d("2026-04-29"))
    })

    it("returns the next Friday when today is Monday (backend=4, JS=5)", () => {
      // Next Friday from 2026-04-27 is 2026-05-01
      const result = computeAnchor({
        mode: "fixed_weekday",
        shoppingDay: 5,
        fixedWeekday: 4, // Friday (backend)
        weekStartsOn: "monday",
        today: MONDAY,
      })
      expect(result).toEqual(d("2026-05-01"))
    })
  })
})

describe("windowRange", () => {
  it("returns inclusive from/to strings 6 days apart for days=7", () => {
    const anchor = d("2026-04-27")
    const { from, to } = windowRange(anchor, 7)
    expect(from).toBe("2026-04-27")
    expect(to).toBe("2026-05-03")
  })

  it("returns a single-day range when days=1", () => {
    const anchor = d("2026-04-27")
    const { from, to } = windowRange(anchor, 1)
    expect(from).toBe("2026-04-27")
    expect(to).toBe("2026-04-27")
  })

  it("does not roll month math wrong near a month boundary", () => {
    // anchor = 2026-01-29, days = 7 → to should be 2026-02-04
    const anchor = d("2026-01-29")
    const { from, to } = windowRange(anchor, 7)
    expect(from).toBe("2026-01-29")
    expect(to).toBe("2026-02-04")
  })

  it("crosses year boundary correctly", () => {
    // anchor = 2025-12-29, days = 7 → to = 2026-01-04
    const anchor = d("2025-12-29")
    const { from, to } = windowRange(anchor, 7)
    expect(from).toBe("2025-12-29")
    expect(to).toBe("2026-01-04")
  })
})
