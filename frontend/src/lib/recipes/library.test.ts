import { beforeEach, describe, expect, it, vi } from "vitest"

import {
  hasLibraryFilters,
  lastEatenAgo,
  libraryQuery,
  parseLibrarySearch,
  rememberedLibrarySearch,
  rememberLibrarySearch,
} from "./library"

describe("rememberLibrarySearch", () => {
  beforeEach(() => sessionStorage.clear())

  it("hands back the last search without the import sheet", () => {
    rememberLibrarySearch({
      q: "chili",
      quick: true,
      sort: "created",
      add: "url",
    })
    expect(rememberedLibrarySearch()).toEqual(
      parseLibrarySearch({ q: "chili", quick: true, sort: "created" })
    )
  })

  it("is empty before the library was visited or after a bad write", () => {
    expect(rememberedLibrarySearch()).toEqual(parseLibrarySearch({}))
    sessionStorage.setItem("plantry:recipes:search", "not json")
    expect(rememberedLibrarySearch()).toEqual(parseLibrarySearch({}))
    sessionStorage.setItem("plantry:recipes:search", "null")
    expect(rememberedLibrarySearch()).toEqual(parseLibrarySearch({}))
  })

  it("never hands back the import sheet, even from a stored add", () => {
    sessionStorage.setItem(
      "plantry:recipes:search",
      JSON.stringify({ q: "chili", add: "url" })
    )
    expect(rememberedLibrarySearch()).toEqual(
      parseLibrarySearch({ q: "chili" })
    )
  })

  it("survives storage that throws on write and read", () => {
    const set = vi
      .spyOn(Storage.prototype, "setItem")
      .mockImplementation(() => {
        throw new DOMException("quota", "QuotaExceededError")
      })
    const get = vi
      .spyOn(Storage.prototype, "getItem")
      .mockImplementation(() => {
        throw new DOMException("denied", "SecurityError")
      })
    try {
      expect(() => rememberLibrarySearch({ q: "chili" })).not.toThrow()
      expect(rememberedLibrarySearch()).toEqual(parseLibrarySearch({}))
    } finally {
      set.mockRestore()
      get.mockRestore()
    }
  })
})

describe("parseLibrarySearch", () => {
  it("keeps known values", () => {
    expect(
      parseLibrarySearch({
        q: "chili",
        role: "main",
        diet: "vegetarian",
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
      diet: "vegetarian",
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
        diet: "omnivore",
        quick: false,
        sort: "kcal",
        add: "discover",
      })
    ).toEqual({
      q: undefined,
      role: undefined,
      diet: undefined,
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
        diet: "vegan",
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
      diet: "vegan",
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
      diet: undefined,
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
    expect(hasLibraryFilters({ diet: "pescetarian" })).toBe(true)
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
