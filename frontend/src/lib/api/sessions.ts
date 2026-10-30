import { apiFetch } from "./client"
import type { RemovedResponse } from "./plates"

/** Where a linked component sits relative to the cook cell (D12). */
export type SessionLinkKind = "fresh" | "leftover" | "frozen"

/** One component that eats from a session, in cell order. */
export interface SessionLink {
  plate_id: number
  component_id: number
  date: string
  slot_id: number
  /** Portions consumed: component portions × plate eaters. */
  portions: number
  kind: SessionLinkKind
  /** 1-based position among the session's links ("2/3"). */
  position: number
}

export type SessionWarningCode =
  | "leftover_before_cook"
  | "past_keep_days"
  | "deficit"
  | "frozen_deficit"

export interface SessionWarning {
  code: SessionWarningCode
  plate_id: number | null
}

export interface Session {
  id: number
  food_id: number
  food_name: string
  cook_date: string
  cook_slot_id: number
  portions: number
  frozen_portions: number
  cooked_at: string | null
  note: string | null
  keep_days: number
  use_by: string
  consumed: number
  frozen_consumed: number
  surplus: number
  links: SessionLink[]
  meals: number
  warnings: SessionWarning[]
}

export interface Cell {
  date: string
  slot_id: number
}

/** PUT /api/cook-sessions/{id} is a full replace: a missing note clears it. */
export interface UpdateSessionInput {
  portions: number
  frozen_portions: number
  note: string | null
}

export function getSession(id: number): Promise<Session> {
  return apiFetch(`/cook-sessions/${id}`)
}

export function updateSession(
  id: number,
  input: UpdateSessionInput
): Promise<Session> {
  return apiFetch(`/cook-sessions/${id}`, {
    method: "PUT",
    body: JSON.stringify(input),
  })
}

export function setSessionCooked(
  id: number,
  cooked: boolean
): Promise<Session> {
  return apiFetch(`/cook-sessions/${id}/cooked`, {
    method: "POST",
    body: JSON.stringify({ cooked }),
  })
}

/** The next `count` eligible leftover cells after the cook cell (D29). */
export function getLeftoverCells(
  id: number,
  count: number
): Promise<{ cells: Cell[] }> {
  return apiFetch(`/cook-sessions/${id}/leftover-cells?count=${count}`)
}

export function addLeftovers(id: number, cells: Cell[]): Promise<Session> {
  return apiFetch(`/cook-sessions/${id}/leftovers`, {
    method: "POST",
    body: JSON.stringify({ cells }),
  })
}

/** "Remove batch": deletes the session, its links and emptied plates. */
export function deleteSession(id: number): Promise<RemovedResponse> {
  return apiFetch(`/cook-sessions/${id}`, { method: "DELETE" })
}
