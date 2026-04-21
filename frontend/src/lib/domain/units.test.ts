import { describe, expect, it } from "vitest"
import {
  availableUnits,
  isCountUnit,
  normalizeUnit,
  resolveGrams,
  unitGroups,
} from "./units"

describe("normalizeUnit", () => {
  it.each([
    ["", ""],
    [" ", ""],
    ["g", "g"],
    ["G", "g"],
    [" g ", "g"],
    ["g.", "g"],
    ["TBSP", "tbsp"],
    ["Tablespoon", "tbsp"],
    ["EL", "tbsp"],
    ["Esslöffel", "tbsp"],
    ["TL", "tsp"],
    ["Zehen", "clove"],
    ["cloves", "clove"],
    ["Stück", "piece"],
    ["unknown-unit", "unknown-unit"],
  ])("normalizes %s → %s", (input, expected) => {
    expect(normalizeUnit(input)).toBe(expected)
  })
})

describe("isCountUnit", () => {
  it("flags count units", () => {
    expect(isCountUnit("clove")).toBe(true)
    expect(isCountUnit("piece")).toBe(true)
    expect(isCountUnit("serving")).toBe(true)
  })
  it("rejects mass/volume units", () => {
    expect(isCountUnit("g")).toBe(false)
    expect(isCountUnit("tbsp")).toBe(false)
  })
})

describe("resolveGrams", () => {
  it("returns exact grams from an ingredient portion", () => {
    const result = resolveGrams(2, "tbsp", [{ unit: "tbsp", grams: 21 }])
    expect(result).toEqual({
      grams: 42,
      source: "portion",
      approximate: false,
      unit: "tbsp",
    })
  })

  it("falls back to a universal volume default (water-density)", () => {
    const result = resolveGrams(2, "tbsp", [])
    expect(result.grams).toBe(30)
    expect(result.source).toBe("fallback")
    expect(result.approximate).toBe(true)
  })

  it("treats water as water-density ml", () => {
    const result = resolveGrams(100, "ml", [])
    expect(result.grams).toBe(100)
    expect(result.source).toBe("fallback")
    expect(result.approximate).toBe(true)
  })

  it("marks bare grams as direct and exact", () => {
    const result = resolveGrams(250, "g", [])
    expect(result.grams).toBe(250)
    expect(result.source).toBe("direct")
    expect(result.approximate).toBe(false)
  })

  it("marks kg as a mass default", () => {
    const result = resolveGrams(1.5, "kg", [])
    expect(result.grams).toBe(1500)
    expect(result.source).toBe("default")
    expect(result.approximate).toBe(false)
  })

  it("reports unresolved for a count unit without portion", () => {
    const result = resolveGrams(2, "clove", [])
    expect(result.grams).toBe(0)
    expect(result.source).toBe("unresolved")
    expect(result.unit).toBe("clove")
  })

  it("accepts a manual grams override for a count unit", () => {
    const result = resolveGrams(2, "clove", [], 10)
    expect(result.grams).toBe(10)
    expect(result.source).toBe("manual")
    expect(result.approximate).toBe(true)
  })

  it("normalizes German aliases before lookup", () => {
    const result = resolveGrams(2, "EL", [])
    expect(result.unit).toBe("tbsp")
    expect(result.grams).toBe(30)
  })
})

describe("availableUnits", () => {
  it("merges defaults with per-ingredient portions and dedupes", () => {
    const units = availableUnits([
      { unit: "tbsp", grams: 21 }, // already in defaults; deduped
      { unit: "clove", grams: 4 }, // custom count unit; appended
    ])
    expect(units).toContain("g")
    expect(units).toContain("tbsp")
    expect(units).toContain("clove")
    // tbsp must appear only once.
    expect(units.filter((u) => u === "tbsp")).toHaveLength(1)
  })
})

describe("unitGroups", () => {
  it("classifies universal units into mass/volume/count", () => {
    const g = unitGroups([])
    expect(g.portions).toEqual([])
    expect(g.mass.map((o) => o.key)).toContain("g")
    expect(g.mass.map((o) => o.key)).toContain("lb")
    expect(g.volume.map((o) => o.key)).toContain("tbsp")
    expect(g.count.map((o) => o.key)).toContain("clove")
  })

  it("lifts ingredient-specific portions into the portions group with grams", () => {
    const g = unitGroups([
      { unit: "tbsp", grams: 21 },
      { unit: "scoop", grams: 35 },
    ])
    expect(g.portions.map((o) => o.key)).toEqual(["tbsp", "scoop"])
    expect(g.portions[0]).toMatchObject({ group: "portions", grams: 21 })
    // tbsp now only appears in the portions group, not in volume.
    expect(g.volume.some((o) => o.key === "tbsp")).toBe(false)
    // scoop is an unknown unit — surfaced as a portion, not duplicated in custom.
    expect(g.custom.some((o) => o.key === "scoop")).toBe(false)
  })
})
