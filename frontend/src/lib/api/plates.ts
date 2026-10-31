import { apiFetch } from "./client"
import type { PlateFeedback } from "./feedback"
import type { FoodKind, FoodRole } from "./foods"

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

/** Draft rows are proposals, confirmed rows are fixed (D1). The state is
 *  uniform per batch closure (D3). */
export type PlanState = "draft" | "confirmed"

export interface Plate {
  id: number
  date: string
  slot_id: number
  /** REAL > 0; defaults to the household portions on the server (D10). */
  eaters: number
  note: string | null
  components: PlateComponent[]
  feedback: PlateFeedback | null
  state: PlanState
  /** The generation run that proposed the plate; null for hand-made ones. */
  ai_run_id: number | null
  /** The chooser's reason ("Why this?"); null for hand-made plates. */
  reason: string | null
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
  /** Optional on input: the actor decides (D2) unless restoring (D5). */
  state?: PlanState
  ai_run_id?: number | null
  reason?: string | null
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
  state?: PlanState
  ai_run_id?: number | null
  reason?: string | null
}

export interface PlanSnapshot {
  plates: SnapshotPlate[]
  sessions: SnapshotSession[]
}

/** The body of every removing mutation: what restores it. */
export interface RemovedResponse {
  removed: PlanSnapshot
}

/** The rows a state change touched: the whole batch closure (D3). */
export interface StateChange {
  plate_ids: number[]
  session_ids: number[]
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

export function createPlate(input: CreatePlateInput): Promise<Plate> {
  return apiFetch("/plates", { method: "POST", body: JSON.stringify(input) })
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

/** POST /api/plates/{id}/confirm: confirms the plate's batch closure. */
export function confirmPlate(id: number): Promise<StateChange> {
  return apiFetch(`/plates/${id}/confirm`, { method: "POST" })
}

/** POST /api/plates/{id}/unlock: returns the plate's closure to draft. */
export function unlockPlate(id: number): Promise<StateChange> {
  return apiFetch(`/plates/${id}/unlock`, { method: "POST" })
}
