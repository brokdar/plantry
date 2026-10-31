import { describe, expect, it } from "vitest"

import { detectTimers, formatRemaining, splitByTimers } from "./timers"

describe("detectTimers", () => {
  it.each([
    // English
    ["Simmer for 5 min.", "5 min", 5 * 60],
    ["Bake 25 minutes until golden", "25 minutes", 25 * 60],
    ["Rest 1 minute", "1 minute", 60],
    ["Braise 2 hours", "2 hours", 2 * 3600],
    ["Roast for 1 hour", "1 hour", 3600],
    ["Cook 1½ hours, covered", "1½ hours", 90 * 60],
    ["Cook 1 ½ hrs", "1 ½ hrs", 90 * 60],
    ["Leave ½ hour", "½ hour", 30 * 60],
    ["Chill 1.5 h", "1.5 h", 90 * 60],
    // Ranges take the upper bound.
    ["Fry 10–12 min", "10–12 min", 12 * 60],
    ["Fry 10-12 mins", "10-12 mins", 12 * 60],
    ["Bake 20 to 25 minutes", "20 to 25 minutes", 25 * 60],
    // German
    ["20 Minuten köcheln lassen", "20 Minuten", 20 * 60],
    ["Ca. 1 Minute anbraten", "1 Minute", 60],
    ["1 Stunde ziehen lassen", "1 Stunde", 3600],
    ["2 Stunden schmoren", "2 Stunden", 2 * 3600],
    ["1,5 Std. im Ofen", "1,5 Std", 90 * 60],
    ["8 Min. kochen", "8 Min", 8 * 60],
    ["10 bis 15 Minuten backen", "10 bis 15 Minuten", 15 * 60],
    ["10–12 Min. garen", "10–12 Min", 12 * 60],
    ["1½ Stunden garen", "1½ Stunden", 90 * 60],
  ])("%s → %s", (text, match, seconds) => {
    const timers = detectTimers(text)
    expect(timers).toHaveLength(1)
    expect(timers[0].text).toBe(match)
    expect(timers[0].seconds).toBe(seconds)
    expect(text.slice(timers[0].start, timers[0].end)).toBe(match)
  })

  it("finds every duration in a step, in order", () => {
    const timers = detectTimers("Fry 5 min, then simmer 20 Minuten.")
    expect(timers.map((t) => t.seconds)).toEqual([300, 1200])
  })

  it.each([
    ["Add 2 onions and 400 g beans"],
    ["Preheat to 200 °C"],
    ["Use 3 tbsp oil"],
    ["Minute steak, seared"],
    ["5 minis on the side"],
    ["Cut into 2 cm pieces"],
    ["Serve with 4 heads of lettuce"],
    ["0 min"],
    [""],
  ])("no timer in %j", (text) => {
    expect(detectTimers(text)).toEqual([])
  })
})

describe("splitByTimers", () => {
  it("splits a step into text and timer parts", () => {
    const parts = splitByTimers("Simmer 20 min, stir.")
    expect(parts).toEqual([
      { kind: "text", text: "Simmer " },
      expect.objectContaining({ kind: "timer", text: "20 min", seconds: 1200 }),
      { kind: "text", text: ", stir." },
    ])
  })

  it("keeps a step without timers whole", () => {
    expect(splitByTimers("Stir.")).toEqual([{ kind: "text", text: "Stir." }])
  })
})

describe("formatRemaining", () => {
  it.each([
    [0, "0:00"],
    [59_400, "1:00"],
    [65_000, "1:05"],
    [20 * 60_000, "20:00"],
    [90 * 60_000, "1:30:00"],
    [-3_000, "0:00"],
  ])("%d ms → %s", (ms, text) => {
    expect(formatRemaining(ms)).toBe(text)
  })
})
