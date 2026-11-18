import { todayISO } from "@/lib/dates"

import { apiFetch } from "./client"
import type { Issue } from "./plan"
import type { Plate, PlanSnapshot } from "./plates"

// Meal operations (meal editing slices 2b/3, backend `handlers/meal_ops.go`):
// each returns what else it changed and a revert token for one Undo.

export interface MealCell {
  date: string
  slot_id: number
}

/** A hand edit's revert token: the plate events it wrote and the
 *  availability cells it changed (`POST /api/plan/revert`). */
export interface Undo {
  first_event_id: number
  last_event_id: number
  cells: {
    date: string
    slot_id: number
    before: unknown
    after: unknown
  }[]
}

export type AlsoChangedWhat =
  | "cook_moved"
  | "became_cooking_meal"
  | "cooks_fresh"
  | "batch_resized"

/** Another meal or batch an edit changed (`plate.AlsoChanged`). */
export interface AlsoChanged {
  session_id: number
  /** The meal now cooking (`cook_moved`, `became_cooking_meal`) or the one
   *  that left the batch (`cooks_fresh`). */
  plate_id?: number
  date: string
  slot_id: number
  what: AlsoChangedWhat
  from_date?: string
  from_slot_id?: number
  why?: "before_cook" | "past_keep_days" | "past_freezer_days" | "cooked"
}

export interface MealResult {
  plates: Plate[]
  session_ids: number[]
  also_changed: AlsoChanged[]
  undo: Undo
}

/** What to do with the plate on a taken target cell. */
export type Occupant = "swap" | "drop" | MealCell

export interface MoveInput extends MealCell {
  occupant?: Occupant
}

/** `dryRun` previews the call: the response is what it would do, nothing
 *  is kept, and its undo token must not be used. */
function q(dryRun: boolean | undefined): string {
  return dryRun ? "?dry_run=1" : ""
}

/** POST /api/plates/{id}/move; a taken target without `occupant` is 409
 *  `error.plate.cell_occupied`. */
export function movePlate(
  id: number,
  input: MoveInput,
  dryRun?: boolean
): Promise<MealResult> {
  return apiFetch(`/plates/${id}/move${q(dryRun)}`, {
    method: "POST",
    body: JSON.stringify(input),
  })
}

export type ReplaceInput = (
  | { position: "main" | "side" }
  | { plate_component_id: number }
) & {
  food_id: number
  session_id?: number
  cook_at?: MealCell
  portions?: number
  amount?: number
  unit?: string
}

/** POST /api/plates/{id}/replace: one component for another food. */
export function replaceComponent(
  id: number,
  input: ReplaceInput,
  dryRun?: boolean
): Promise<MealResult> {
  return apiFetch(`/plates/${id}/replace${q(dryRun)}`, {
    method: "POST",
    body: JSON.stringify(input),
  })
}

/** POST /api/plates/{id}/copy: all cells or none. */
export function copyPlate(
  id: number,
  cells: MealCell[],
  as: "fresh" | "leftover"
): Promise<MealResult> {
  return apiFetch(`/plates/${id}/copy`, {
    method: "POST",
    body: JSON.stringify({ cells, as }),
  })
}

/** POST /api/cook-sessions/{id}/move-cook: refused (400) when a meal
 *  would come before the cook or past its keeping time. */
export function moveCook(id: number, cell: MealCell): Promise<MealResult> {
  return apiFetch(`/cook-sessions/${id}/move-cook`, {
    method: "POST",
    body: JSON.stringify(cell),
  })
}

export type AwayInput = { cells: MealCell[] } | { date: string }

export interface AwayResult {
  removed: PlanSnapshot | null
  also_changed: AlsoChanged[]
  undo: Undo
}

/** POST /api/availability/away: the cells (or every slot of `date`) become
 *  No plate and their plates go, in one change. */
export function setAway(
  input: AwayInput,
  dryRun?: boolean
): Promise<AwayResult> {
  const qs = new URLSearchParams({ today: todayISO() })
  if (dryRun) qs.set("dry_run", "1")
  return apiFetch(`/availability/away?${qs}`, {
    method: "POST",
    body: JSON.stringify(input),
  })
}

/** POST /api/plan/revert: takes a revert token back; 409
 *  `error.plan.change_stale` when a row was edited since. */
export function revertPlan(
  undo: Undo
): Promise<{ plate_ids: number[]; session_ids: number[] }> {
  return apiFetch("/plan/revert", {
    method: "POST",
    body: JSON.stringify(undo),
  })
}

// ── Alternatives (slice 3a) ──────────────────────────────────────────

export type AlternativeWhat = "meal" | "main" | "side" | "add"

export interface AlternativeConstraints {
  max_minutes?: number
  diet?: string
  min_protein_g?: number
  max_kcal?: number
}

export interface AlternativesInput {
  plate_id: number
  what: AlternativeWhat
  plate_component_id?: number
  constraints?: AlternativeConstraints
}

/** One `apply_changes` op of an alternative, in order. */
export type AlternativeOp =
  | {
      op: "swap"
      plate_component_id: number
      food_id: number
      portions?: number
      amount?: number
      unit?: string
    }
  | {
      op: "add"
      plate_id?: number
      food_id: number
      session_id?: number
      cook_at?: MealCell
      portions?: number
      amount?: number
      unit?: string
      position?: string
    }
  | {
      /** A cook_at alternative: the food cooks on `cook_at` and takes the
       *  component's place, one `POST /api/plates/{id}/replace`. */
      op: "replace"
      plate_id?: number
      plate_component_id: number
      food_id: number
      cook_at: MealCell
      portions?: number
      amount?: number
      unit?: string
    }
  | { op: "remove"; plate_component_id: number }

export interface Alternative {
  food_id: number
  name: string
  source: "fresh" | "session" | "cook_at" | "leaf"
  session_id?: number
  /** The cell a cook_at alternative cooks on (it shares that cook). */
  cook_at?: MealCell
  total_minutes?: number
  kcal: number
  protein_g: number
  day_kcal: number
  day_pct?: number
  week_pct?: number
  cook_occasions_day: number
  new_issues?: Issue[]
  ops: AlternativeOp[]
}

/** Why a list is empty: how many candidates fit the position and the
 *  filters, and how many each rule dropped (`"H2.cook_cap"`: rule.code). */
export interface EmptyReason {
  candidates: number
  dropped?: Record<string, number>
}

export interface AlternativesResult {
  alternatives: Alternative[]
  empty_reason?: EmptyReason
}

/** POST /api/plan/alternatives?start&today: ranked, writes nothing. */
export function getAlternatives(
  start: string,
  input: AlternativesInput
): Promise<AlternativesResult> {
  const qs = new URLSearchParams({ start, today: todayISO() })
  return apiFetch(`/plan/alternatives?${qs}`, {
    method: "POST",
    body: JSON.stringify(input),
  })
}

// ── Regenerate day preview (slice 3b) ────────────────────────────────

export interface RegeneratePreview {
  replaced: number[]
  also_changed: AlsoChanged[]
  confirmed_affected: number[]
}

/** GET /api/plan/regenerate/preview?scope=day: what the run's removal does. */
export function getRegeneratePreview(
  start: string,
  date: string
): Promise<RegeneratePreview> {
  const qs = new URLSearchParams({
    scope: "day",
    date,
    start,
    today: todayISO(),
  })
  return apiFetch(`/plan/regenerate/preview?${qs}`)
}
