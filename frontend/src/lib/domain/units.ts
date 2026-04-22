// Canonical unit vocabulary for recipe ingredients. Mirrors the backend
// package `backend/internal/domain/units`. When adding a unit, update both
// sides.

export type UnitKind = "mass" | "volume" | "count"

export type GramsSource =
  | "direct" // unit is a bare mass (g); exact
  | "portion" // matched an ingredient-specific portion (FDC/OFF/manual)
  | "default" // universal mass default (e.g., oz, kg)
  | "fallback" // universal volume default (water-density assumption)
  | "manual" // user-supplied grams override for an unresolved unit
  | "unresolved" // count unit without portion and without manual grams

export interface UnitDefault {
  grams: number
  kind: UnitKind
  approximate: boolean
}

export const UNIT_DEFAULTS: Record<string, UnitDefault> = {
  g: { grams: 1, kind: "mass", approximate: false },
  kg: { grams: 1000, kind: "mass", approximate: false },
  mg: { grams: 0.001, kind: "mass", approximate: false },
  oz: { grams: 28.3495, kind: "mass", approximate: false },
  lb: { grams: 453.592, kind: "mass", approximate: false },
  ml: { grams: 1, kind: "volume", approximate: true },
  l: { grams: 1000, kind: "volume", approximate: true },
  cl: { grams: 10, kind: "volume", approximate: true },
  dl: { grams: 100, kind: "volume", approximate: true },
  tbsp: { grams: 15, kind: "volume", approximate: true },
  tsp: { grams: 5, kind: "volume", approximate: true },
  cup: { grams: 240, kind: "volume", approximate: true },
  floz: { grams: 29.5735, kind: "volume", approximate: true },
  pt: { grams: 473.176, kind: "volume", approximate: true },
  qt: { grams: 946.353, kind: "volume", approximate: true },
  gal: { grams: 3785.41, kind: "volume", approximate: true },
}

export const COUNT_UNITS = new Set([
  "piece",
  "clove",
  "slice",
  "bunch",
  "pinch",
  "stick",
  "can",
  "jar",
  "packet",
  "stalk",
  "pod",
  "head",
  "leaf",
  "leaves",
  "sprig",
  "serving", // treated as count so a serving without grams is clearly unresolved
])

const ALIASES: Record<string, string> = {
  // Mass
  g: "g",
  gr: "g",
  gram: "g",
  grams: "g",
  gramm: "g",
  kg: "kg",
  kilogram: "kg",
  mg: "mg",
  oz: "oz",
  ounce: "oz",
  ounces: "oz",
  lb: "lb",
  lbs: "lb",
  pound: "lb",
  pounds: "lb",
  // Volume
  ml: "ml",
  milliliter: "ml",
  millilitre: "ml",
  milliliters: "ml",
  l: "l",
  liter: "l",
  litre: "l",
  liters: "l",
  cl: "cl",
  dl: "dl",
  tbsp: "tbsp",
  tb: "tbsp",
  tbs: "tbsp",
  tablespoon: "tbsp",
  tablespoons: "tbsp",
  el: "tbsp",
  essl: "tbsp",
  esslöffel: "tbsp",
  tsp: "tsp",
  ts: "tsp",
  teaspoon: "tsp",
  teaspoons: "tsp",
  tl: "tsp",
  teel: "tsp",
  teelöffel: "tsp",
  cup: "cup",
  cups: "cup",
  floz: "floz",
  "fl oz": "floz",
  fluidounce: "floz",
  pt: "pt",
  pint: "pt",
  pints: "pt",
  qt: "qt",
  quart: "qt",
  quarts: "qt",
  gal: "gal",
  gallon: "gal",
  gallons: "gal",
  // Count
  piece: "piece",
  pieces: "piece",
  pc: "piece",
  pcs: "piece",
  stk: "piece",
  stück: "piece",
  stueck: "piece",
  clove: "clove",
  cloves: "clove",
  zehe: "clove",
  zehen: "clove",
  slice: "slice",
  slices: "slice",
  scheibe: "slice",
  scheiben: "slice",
  bunch: "bunch",
  bunches: "bunch",
  bund: "bunch",
  pinch: "pinch",
  pinches: "pinch",
  prise: "pinch",
  prisen: "pinch",
  stick: "stick",
  sticks: "stick",
  stange: "stick",
  stangen: "stick",
  can: "can",
  cans: "can",
  dose: "can",
  dosen: "can",
  jar: "jar",
  jars: "jar",
  glas: "jar",
  gläser: "jar",
  packet: "packet",
  packets: "packet",
  pck: "packet",
  päckchen: "packet",
  packung: "packet",
  stalk: "stalk",
  stalks: "stalk",
  pod: "pod",
  pods: "pod",
  head: "head",
  heads: "head",
  leaf: "leaf",
  leaves: "leaves",
  sprig: "sprig",
  sprigs: "sprig",
  serving: "serving",
  servings: "serving",
  portion: "serving",
  portions: "serving",
}

/** Normalize a raw unit string to its canonical key. */
export function normalizeUnit(unit: string): string {
  if (!unit) return ""
  const t = unit.trim().toLowerCase().replace(/\.$/, "")
  if (!t) return ""
  return ALIASES[t] ?? t
}

export function isCountUnit(canonical: string): boolean {
  return COUNT_UNITS.has(canonical)
}

export interface PortionLookup {
  unit: string
  grams: number
}

export interface ResolvedGrams {
  grams: number
  source: GramsSource
  approximate: boolean
  unit: string // canonical
}

/**
 * Resolves an amount + unit to grams using the same precedence the backend
 * uses:
 *   1. ingredient-specific portion (exact)
 *   2. universal default (mass = exact, volume = water-density approx)
 *   3. user-supplied `manualGrams` (count/unknown fallback)
 *   4. unresolved (count unit without portion, no manual override)
 *
 * The `approximate` flag is true for volume defaults and manual overrides —
 * signals to the UI that the value is a best guess.
 */
export function resolveGrams(
  amount: number,
  rawUnit: string,
  portions: PortionLookup[] = [],
  manualGrams?: number
): ResolvedGrams {
  const unit = normalizeUnit(rawUnit)
  if (!unit) {
    return {
      grams: 0,
      source: "unresolved",
      approximate: true,
      unit: "",
    }
  }

  // 1. Ingredient-specific portion (skip for bare mass).
  if (unit !== "g" && unit !== "kg" && unit !== "mg") {
    for (const p of portions) {
      if (normalizeUnit(p.unit) === unit) {
        return {
          grams: amount * p.grams,
          source: "portion",
          approximate: false,
          unit,
        }
      }
    }
  }

  // 2. Universal default.
  const def = UNIT_DEFAULTS[unit]
  if (def) {
    let source: GramsSource
    if (def.kind === "mass" && unit === "g") source = "direct"
    else if (def.kind === "mass") source = "default"
    else source = "fallback"
    return {
      grams: amount * def.grams,
      source,
      approximate: def.approximate,
      unit,
    }
  }

  // 3. Manual override for count/unknown units.
  if (manualGrams !== undefined && manualGrams > 0) {
    return {
      grams: manualGrams,
      source: "manual",
      approximate: true,
      unit,
    }
  }

  // 4. Unresolved.
  return {
    grams: 0,
    source: "unresolved",
    approximate: true,
    unit,
  }
}

export type UnitGroup = "portions" | "mass" | "volume" | "count" | "custom"

export interface UnitOption {
  key: string
  group: UnitGroup
  /** Grams per one unit — only populated for ingredient-specific portions. */
  grams?: number
}

const MASS_UNITS = ["g", "kg", "mg", "oz", "lb"] as const
const VOLUME_UNITS = [
  "ml",
  "l",
  "cl",
  "dl",
  "tbsp",
  "tsp",
  "cup",
  "floz",
] as const
const COUNT_UNITS_ORDERED = [
  "piece",
  "clove",
  "slice",
  "bunch",
  "pinch",
  "stick",
  "can",
  "jar",
  "packet",
  "serving",
] as const

/**
 * Partitions unit options into labelled groups for a grouped dropdown.
 * Ingredient-specific portions come first (most relevant), then mass → volume
 * → count. Anything unknown lands in `custom` so it is still reachable but
 * clearly separated.
 */
export function unitGroups(portions: PortionLookup[] = []): {
  portions: UnitOption[]
  mass: UnitOption[]
  volume: UnitOption[]
  count: UnitOption[]
  custom: UnitOption[]
} {
  const seen = new Set<string>()
  const portionOpts: UnitOption[] = []
  for (const p of portions) {
    const key = normalizeUnit(p.unit)
    if (!key || seen.has(key)) continue
    seen.add(key)
    portionOpts.push({ key, group: "portions", grams: p.grams })
  }

  const mass: UnitOption[] = []
  for (const key of MASS_UNITS) {
    if (seen.has(key)) continue
    seen.add(key)
    mass.push({ key, group: "mass" })
  }

  const volume: UnitOption[] = []
  for (const key of VOLUME_UNITS) {
    if (seen.has(key)) continue
    seen.add(key)
    volume.push({ key, group: "volume" })
  }

  const count: UnitOption[] = []
  for (const key of COUNT_UNITS_ORDERED) {
    if (seen.has(key)) continue
    seen.add(key)
    count.push({ key, group: "count" })
  }

  // Any additional portion keys beyond the canonical vocabulary (e.g. a
  // user-created "scoop") that didn't already register as portions above.
  const custom: UnitOption[] = []
  for (const p of portions) {
    const key = normalizeUnit(p.unit)
    if (!key || seen.has(key)) continue
    if (UNIT_DEFAULTS[key] || COUNT_UNITS.has(key)) continue
    seen.add(key)
    custom.push({ key, group: "custom", grams: p.grams })
  }

  return { portions: portionOpts, mass, volume, count, custom }
}

/**
 * Flat list of canonical unit keys — kept for callers that just need a set of
 * available unit strings (tests, legacy consumers). Prefer `unitGroups` for
 * rendering a grouped picker.
 */
export function availableUnits(portions: PortionLookup[] = []): string[] {
  const g = unitGroups(portions)
  return [...g.portions, ...g.mass, ...g.volume, ...g.count, ...g.custom].map(
    (o) => o.key
  )
}
