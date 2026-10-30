import { apiFetch } from "./client"
import type { PlateFeedback } from "./feedback"
import type { FoodKind, FoodRole } from "./foods"

export interface MacrosResponse {
  kcal: number
  protein: number
  fat: number
  carbs: number
  fiber: number
  sodium: number
}

// Week is kept as an in-memory aggregate for plate-patch helpers used by
// optimistic component mutations (swap, add, remove). It is not fetched from
// the server; the week cache slots are populated by the mutation hooks only.
export interface Week {
  id: number
  year: number
  week_number: number
  plates: Plate[]
  created_at: string
}

export interface PlateComponent {
  id: number
  plate_id: number
  food_id: number
  food_name: string
  kind: FoodKind
  role?: FoodRole | null
  /** Composed: portions per eater (REAL, usually 1). */
  portions?: number
  /** Composed: the cook session this component eats from (D8). */
  cook_session_id?: number
  /** Leaf: per-eater quantity in `unit`. */
  amount?: number
  /** Leaf: canonical unit key (g, ml, piece…). */
  unit?: string
  /** Leaf: server-resolved grams per eater. */
  grams?: number
  /** Leaf: confidence label for the grams resolution. */
  grams_source?: string
  sort_order: number
}

export interface Plate {
  id: number
  date: string
  slot_id: number
  /** REAL > 0; defaults to the household portions on the server (D10). */
  eaters: number
  note: string | null
  components: PlateComponent[]
  feedback: PlateFeedback | null
  created_at: string
  updated_at: string
}

/** ComponentInput of §3.1: composed → portions per eater (default 1),
 *  optionally eating from an existing session; leaf → amount + unit. */
export type ComponentInput =
  | { food_id: number; portions?: number; cook_session_id?: number }
  | { food_id: number; amount: number; unit: string }

export interface CreatePlateInput {
  date: string
  slot_id: number
  eaters?: number
  note?: string | null
  components?: ComponentInput[]
}

export interface UpdatePlateInput {
  date?: string
  slot_id?: number
  eaters?: number
  note?: string | null
}

// ── PlanSnapshot (D22): the only undo and copy format ─────────────────

/** A composed component's session: `ref` names a session in the same
 *  snapshot ("s1"), `id` one that still exists. */
export type SnapshotSessionLink = { ref: string } | { id: number }

export interface SnapshotComponent {
  food_id: number
  portions?: number
  amount?: number
  unit?: string
  grams?: number
  sort_order: number
  session?: SnapshotSessionLink
}

export interface SnapshotPlate {
  /** Set when the plate survived and only some components were removed. */
  plate_id?: number
  date: string
  slot_id: number
  eaters: number
  note: string | null
  feedback?: PlateFeedback
  components: SnapshotComponent[]
}

export interface SnapshotSession {
  ref: string
  food_id: number
  cook_date: string
  cook_slot_id: number
  portions: number
  frozen_portions: number
  cooked_at: string | null
  note: string | null
}

export interface PlanSnapshot {
  plates: SnapshotPlate[]
  sessions: SnapshotSession[]
}

/** The body of every removing mutation: what restores it. */
export interface RemovedResponse {
  removed: PlanSnapshot
}

/**
 * UpdatePlateComponentInput supports two modes:
 *  - swap: `food_id` set (with optional new quantity for the new food)
 *  - quantity update: only `portions` OR `amount + unit`
 */
export interface UpdatePlateComponentInput {
  food_id?: number
  portions?: number
  amount?: number
  unit?: string
}

/**
 * PlateComponentQuantity is the quantity-only patch shape used by quantity
 * updates and optimistic patches. Discriminated by the keys present.
 */
export type PlateComponentQuantity =
  | { portions: number }
  | { amount: number; unit: string }

/**
 * componentMultiplier mirrors the backend `PlateComponent.Multiplier` rule:
 *   - composed → portions
 *   - leaf     → grams / 100
 *   - missing both → null (caller should render a placeholder)
 *
 * Use this anywhere the UI needs to scale per-portion (composed) or
 * per-100 g (leaf) macros for a plate component.
 */
export function componentMultiplier(pc: PlateComponent): number | null {
  if (pc.portions != null) return pc.portions
  if (pc.grams != null) return pc.grams / 100
  return null
}

/**
 * componentToAddInput projects an existing PlateComponent into the kind-aware
 * `ComponentInput` shape used when copying / templating / cloning a
 * plate. Composed components carry their integer portions; leaf components
 * carry the user-entered amount + unit (grams will be re-resolved
 * server-side at write time, mirroring the original create path).
 *
 * Falls back to `{ portions: 1 }` when neither shape is set — that should
 * never happen against a real backend response but keeps the helper total.
 */
export function componentToAddInput(pc: PlateComponent): ComponentInput {
  if (pc.portions != null) {
    return { food_id: pc.food_id, portions: pc.portions }
  }
  if (pc.amount != null && pc.unit != null) {
    return { food_id: pc.food_id, amount: pc.amount, unit: pc.unit }
  }
  return { food_id: pc.food_id, portions: 1 }
}

export function listPlates(
  from: string,
  to: string
): Promise<{ plates: Plate[] }> {
  return apiFetch(
    `/plates?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`
  )
}

export function createPlate(input: CreatePlateInput): Promise<Plate> {
  return apiFetch("/plates", { method: "POST", body: JSON.stringify(input) })
}

export function getPlate(id: number): Promise<Plate> {
  return apiFetch(`/plates/${id}`)
}

export function updatePlate(
  id: number,
  input: UpdatePlateInput
): Promise<Plate> {
  return apiFetch(`/plates/${id}`, {
    method: "PUT",
    body: JSON.stringify(input),
  })
}

export function deletePlate(id: number): Promise<RemovedResponse> {
  return apiFetch(`/plates/${id}`, { method: "DELETE" })
}

export function addPlateComponent(
  plateId: number,
  input: ComponentInput
): Promise<Plate> {
  return apiFetch(`/plates/${plateId}/components`, {
    method: "POST",
    body: JSON.stringify(input),
  })
}

export function updatePlateComponent(
  plateId: number,
  pcId: number,
  input: UpdatePlateComponentInput
): Promise<Plate> {
  return apiFetch(`/plates/${plateId}/components/${pcId}`, {
    method: "PUT",
    body: JSON.stringify(input),
  })
}

export function deletePlateComponent(
  plateId: number,
  pcId: number
): Promise<Plate> {
  return apiFetch(`/plates/${plateId}/components/${pcId}`, { method: "DELETE" })
}

export interface PlateMacrosEntry {
  plate_id: number
  date: string
  macros: MacrosResponse
}

/** GET /api/plates/macros?from=&to= — per-plate macro totals for a date range. */
export function listPlateMacros(
  from: string,
  to: string
): Promise<{ plates: PlateMacrosEntry[] }> {
  return apiFetch(
    `/plates/macros?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`
  )
}
