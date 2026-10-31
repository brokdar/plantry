import type { DietClass, FoodListParams, FoodRole } from "@/lib/api/foods"
import { FOOD_ROLES } from "@/lib/schemas/food"

/** The Diet chip's choices. Each keeps recipes at most that class
 *  ("Vegetarian" includes vegan ones); "with meat" would only drop the
 *  unknown ones, so it is not offered. */
export const DIET_FILTERS = [
  "vegan",
  "vegetarian",
  "pescetarian",
] as const satisfies readonly DietClass[]
export type DietFilter = (typeof DIET_FILTERS)[number]

export const LIBRARY_SORTS = ["name", "last_eaten", "created"] as const
export type LibrarySort = (typeof LIBRARY_SORTS)[number]

/** The library's filters, kept in the `/recipes` search params so a filtered
 *  view is shareable and survives a reload (F8). `add=url` opens the import
 *  sheet. */
export interface LibrarySearch {
  q?: string
  role?: FoodRole
  /** At most this diet class (D28). */
  diet?: DietFilter
  /** ≤ 30 min prep + cook. */
  quick?: true
  /** Not eaten in the last 4 weeks. */
  stale?: true
  fav?: true
  /** Added in the last 30 days. */
  new?: true
  sort?: LibrarySort
  add?: "url"
}

export const QUICK_MINUTES = 30
export const STALE_WEEKS = 4
export const NEW_DAYS = 30

const flag = (v: unknown): true | undefined =>
  v === true || v === "true" ? true : undefined

const oneOf = <T extends string>(
  values: readonly T[],
  v: unknown
): T | undefined =>
  typeof v === "string" && (values as readonly string[]).includes(v)
    ? (v as T)
    : undefined

/** Drops anything unknown, so a hand-edited URL can't break the list. */
export function parseLibrarySearch(
  raw: Record<string, unknown>
): LibrarySearch {
  // The router JSON-parses search values, so a typed `?q=123` arrives as a
  // number.
  const q =
    typeof raw.q === "string" || typeof raw.q === "number"
      ? String(raw.q) || undefined
      : undefined
  return {
    q,
    role: oneOf(FOOD_ROLES, raw.role),
    diet: oneOf(DIET_FILTERS, raw.diet),
    quick: flag(raw.quick),
    stale: flag(raw.stale),
    fav: flag(raw.fav),
    new: flag(raw.new),
    sort: oneOf(LIBRARY_SORTS, raw.sort),
    add: raw.add === "url" ? "url" : undefined,
  }
}

const SEARCH_KEY = "plantry:recipes:search"

/** Keeps the library's last search for the tab's session, so "Back to
 *  recipes" returns to the same view. The import sheet is not kept. Full or
 *  disabled storage only loses the memory. */
export function rememberLibrarySearch(s: LibrarySearch): void {
  try {
    sessionStorage.setItem(SEARCH_KEY, JSON.stringify({ ...s, add: undefined }))
  } catch {
    // Back falls back to the unfiltered library.
  }
}

/** The search `rememberLibrarySearch` kept, never with the import sheet, or
 *  none when nothing (readable) was kept. */
export function rememberedLibrarySearch(): LibrarySearch {
  try {
    const raw: unknown = JSON.parse(sessionStorage.getItem(SEARCH_KEY) ?? "{}")
    const search = parseLibrarySearch(
      raw && typeof raw === "object" ? (raw as Record<string, unknown>) : {}
    )
    return { ...search, add: undefined }
  } catch {
    return parseLibrarySearch({})
  }
}

/** True when any filter (not the sort) narrows the list. */
export function hasLibraryFilters(s: LibrarySearch): boolean {
  return !!(s.q || s.role || s.diet || s.quick || s.stale || s.fav || s.new)
}

/** Maps the filters onto `GET /api/foods` (B7 list params). */
export function libraryQuery(s: LibrarySearch): FoodListParams {
  return {
    kind: "composed",
    search: s.q,
    role: s.role,
    diet: s.diet,
    max_minutes: s.quick ? QUICK_MINUTES : undefined,
    not_eaten_weeks: s.stale ? STALE_WEEKS : undefined,
    favorite: s.fav ? 1 : undefined,
    created_within_days: s.new ? NEW_DAYS : undefined,
    sort: s.sort ?? "name",
  }
}

const DAY_MS = 86_400_000

function dayNumber(ymd: string): number {
  const [y, m, d] = ymd.split("-").map(Number)
  return Date.UTC(y, m - 1, d) / DAY_MS
}

/** How long ago a recipe was last eaten, as a localized relative time
 *  ("3 weeks ago", "yesterday"), or null when it was never eaten. */
export function lastEatenAgo(
  lastEaten: string | null,
  today: string,
  lang: string
): string | null {
  if (!lastEaten) return null
  const days = Math.max(0, dayNumber(today) - dayNumber(lastEaten))
  const rtf = new Intl.RelativeTimeFormat(lang, { numeric: "auto" })
  if (days < 7) return rtf.format(-days, "day")
  if (days < 60) return rtf.format(-Math.floor(days / 7), "week")
  if (days < 365) return rtf.format(-Math.floor(days / 30), "month")
  return rtf.format(-Math.floor(days / 365), "year")
}
