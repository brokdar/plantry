import { describe, expect, it } from "vitest"
import {
  computeAnchor,
  monthGridRange,
  parseYearMonth,
  shiftYMD,
  toYMD,
  weekStartDate,
  windowRange,
} from "./planner-window"

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

describe("toYMD", () => {
  it("formats a date to YYYY-MM-DD", () => {
    expect(toYMD(new Date(2026, 3, 27))).toBe("2026-04-27")
  })

  it("zero-pads month and day", () => {
    expect(toYMD(new Date(2026, 0, 5))).toBe("2026-01-05")
  })
})

describe("parseYearMonth", () => {
  it("parses YYYY-MM to year and 0-based month", () => {
    expect(parseYearMonth("2026-04")).toEqual({ year: 2026, month: 3 })
  })

  it("parses January correctly (month 0)", () => {
    expect(parseYearMonth("2026-01")).toEqual({ year: 2026, month: 0 })
  })
})

describe("weekStartDate", () => {
  it("returns a YYYY-MM-DD string", () => {
    const result = weekStartDate(1)
    expect(result).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })

  it("returned date is always a Monday when weekStartsOn=1", () => {
    const result = weekStartDate(1)
    const [y, m, day] = result.split("-").map(Number)
    const dow = new Date(y, m - 1, day).getDay()
    expect(dow).toBe(1) // Monday
  })

  it("returned date is always a Sunday when weekStartsOn=0", () => {
    const result = weekStartDate(0)
    const [y, m, day] = result.split("-").map(Number)
    const dow = new Date(y, m - 1, day).getDay()
    expect(dow).toBe(0) // Sunday
  })
})

describe("monthGridRange", () => {
  it("grid starts on a Monday for April 2026 when weekStartsOn=1", () => {
    // April 2026: first day is Wed (JS=3); nearest Monday back is 2026-03-30
    const { from, to } = monthGridRange(2026, 3, 1)
    expect(from).toBe("2026-03-30")
    // 5 weeks × 7 = 35 days grid → last day 2026-05-03
    expect(to).toBe("2026-05-03")
  })

  it("grid starts on a Sunday for April 2026 when weekStartsOn=0", () => {
    // April 2026 first is Wed (JS=3); nearest Sunday back is 2026-03-29
    const { from } = monthGridRange(2026, 3, 0)
    expect(from).toBe("2026-03-29")
  })

  it("grid covers the entire month", () => {
    const { from, to } = monthGridRange(2026, 3, 1) // April 2026
    const fromDate = new Date(from)
    const toDate = new Date(to)
    expect(fromDate <= new Date(2026, 3, 1)).toBe(true)
    expect(toDate >= new Date(2026, 3, 30)).toBe(true)
  })

  it("returns a multiple of 7 days", () => {
    const { from, to } = monthGridRange(2026, 0, 1) // January 2026
    const fromDate = new Date(from)
    const toDate = new Date(to)
    const days =
      (toDate.getTime() - fromDate.getTime()) / (1000 * 60 * 60 * 24) + 1
    expect(days % 7).toBe(0)
  })
})

describe("shiftYMD", () => {
  it("shifts forward by 7", () => {
    expect(shiftYMD("2026-04-27", 7)).toBe("2026-05-04")
  })

  it("shifts backward by 7", () => {
    expect(shiftYMD("2026-04-27", -7)).toBe("2026-04-20")
  })

  it("crosses month boundary forward", () => {
    expect(shiftYMD("2026-01-29", 7)).toBe("2026-02-05")
  })

  it("crosses year boundary backward", () => {
    expect(shiftYMD("2026-01-03", -7)).toBe("2025-12-27")
  })

  it("zero shift is a no-op", () => {
    expect(shiftYMD("2026-04-27", 0)).toBe("2026-04-27")
  })
})
