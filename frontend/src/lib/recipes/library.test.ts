import { describe, expect, it } from "vitest"

import {
  hasLibraryFilters,
  lastEatenAgo,
  libraryQuery,
  parseLibrarySearch,
} from "./library"

describe("parseLibrarySearch", () => {
  it("keeps known values", () => {
    expect(
      parseLibrarySearch({
        q: "chili",
        role: "main",
        quick: true,
        stale: "true",
        fav: true,
        new: true,
        sort: "last_eaten",
        add: "url",
      })
    ).toEqual({
      q: "chili",
      role: "main",
      quick: true,
      stale: true,
      fav: true,
      new: true,
      sort: "last_eaten",
      add: "url",
    })
  })

  it("drops unknown or malformed values", () => {
    expect(
      parseLibrarySearch({
        q: "",
        role: "breakfast",
        quick: false,
        sort: "kcal",
        add: "discover",
      })
    ).toEqual({
      q: undefined,
      role: undefined,
      quick: undefined,
      stale: undefined,
      fav: undefined,
      new: undefined,
      sort: undefined,
      add: undefined,
    })
  })
})

describe("parseLibrarySearch q", () => {
  it("keeps a numeric search as text", () => {
    expect(parseLibrarySearch({ q: 123 }).q).toBe("123")
  })
})

describe("libraryQuery", () => {
  it("maps every chip onto the B7 list params", () => {
    expect(
      libraryQuery({
        q: "soup",
        role: "side_veg",
        quick: true,
        stale: true,
        fav: true,
        new: true,
        sort: "created",
      })
    ).toEqual({
      kind: "composed",
      search: "soup",
      role: "side_veg",
      max_minutes: 30,
      not_eaten_weeks: 4,
      favorite: 1,
      created_within_days: 30,
      sort: "created",
    })
  })

  it("defaults to name order with no filters", () => {
    expect(libraryQuery({})).toEqual({
      kind: "composed",
      search: undefined,
      role: undefined,
      max_minutes: undefined,
      not_eaten_weeks: undefined,
      favorite: undefined,
      created_within_days: undefined,
      sort: "name",
    })
  })
})

describe("hasLibraryFilters", () => {
  it("ignores the sort and the import sheet", () => {
    expect(hasLibraryFilters({ sort: "created", add: "url" })).toBe(false)
    expect(hasLibraryFilters({ fav: true })).toBe(true)
  })
})

describe("lastEatenAgo", () => {
  it.each([
    ["2026-10-31", "today"],
    ["2026-10-30", "yesterday"],
    ["2026-10-28", "3 days ago"],
    ["2026-10-10", "3 weeks ago"],
    ["2026-08-01", "3 months ago"],
    ["2024-10-01", "2 years ago"],
  ])("%s → %s", (date, text) => {
    expect(lastEatenAgo(date, "2026-10-31", "en")).toBe(text)
  })

  it("is null when never eaten", () => {
    expect(lastEatenAgo(null, "2026-10-31", "en")).toBeNull()
  })

  it("speaks German", () => {
    expect(lastEatenAgo("2026-10-10", "2026-10-31", "de")).toBe("vor 3 Wochen")
  })
})
