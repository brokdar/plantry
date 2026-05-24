// Unit vocabulary for recipe ingredients. The canonical unit list is served
// by the backend at GET /api/units. Call `initUnitsVocabulary` once (e.g. in
// the app root after the units query resolves) to populate the module-level
// state used by `resolveGrams`, `unitGroups`, and friends.

import type { UnitDescriptor } from "@/lib/api/units"

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

// Module-level vocabulary — populated via initUnitsVocabulary().
export let UNIT_DEFAULTS: Record<string, UnitDefault> = {}
export let COUNT_UNITS: Set<string> = new Set()
let _massUnits: string[] = []
let _volumeUnits: string[] = []
let _countUnitsOrdered: string[] = []

/**
 * Populate the module-level unit vocabulary from the backend's canonical list.
 * Call once after `useUnits()` data is available (typically in the app root).
 */
export function initUnitsVocabulary(descriptors: UnitDescriptor[]): void {
  const defaults: Record<string, UnitDefault> = {}
  const countSet = new Set<string>()
  const mass: string[] = []
  const volume: string[] = []
  const count: string[] = []

  for (const d of descriptors) {
    if (d.group === "mass" || d.group === "volume") {
      defaults[d.id] = {
        grams: d.grams ?? 0,
        kind: d.group,
        approximate: d.approximate ?? false,
      }
      if (d.group === "mass") mass.push(d.id)
      else volume.push(d.id)
    } else {
      countSet.add(d.id)
      count.push(d.id)
    }
  }

  UNIT_DEFAULTS = defaults
  COUNT_UNITS = countSet
  _massUnits = mass
  _volumeUnits = volume
  _countUnitsOrdered = count
}

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
  for (const key of _massUnits) {
    if (seen.has(key)) continue
    seen.add(key)
    mass.push({ key, group: "mass" })
  }

  const volume: UnitOption[] = []
  for (const key of _volumeUnits) {
    if (seen.has(key)) continue
    seen.add(key)
    volume.push({ key, group: "volume" })
  }

  const count: UnitOption[] = []
  for (const key of _countUnitsOrdered) {
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
