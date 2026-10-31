import { todayISO } from "@/lib/dates"

import type { AvailabilityMode, AvailabilitySource } from "./availability"
import { apiFetch } from "./client"
import type { Chooser } from "./generation"
import type {
  Plate,
  PlanSnapshot,
  RemovedResponse,
  StateChange,
} from "./plates"
import type { Session, SessionWarningCode } from "./sessions"

export interface PlanWindow {
  start: string
  end: string
  today: string
  is_current: boolean
  prev_start: string
  next_start: string
  /** 0 = Monday … 6 = Sunday. */
  shopping_day: number
}

export interface PlanSlot {
  id: number
  name_key: string
  icon: string
  sort_order: number
}

export interface PlanCell {
  date: string
  slot_id: number
  mode: AvailabilityMode
  source: AvailabilitySource
  preset_id: number | null
  preset_name: string | null
  note: string | null
  plate: Plate | null
}

export interface DayMacros {
  kcal: number
  protein: number
  fat: number
  carbs: number
}

/** Per-eater nutrition for one day. */
export interface PlanDay {
  date: string
  macros: DayMacros
  complete: boolean
  missing: string[]
}

export interface PlanTargets {
  kcal: number
  protein_g: number
  fat_g: number
  carbs_g: number
}

export interface PlanStats {
  plates: number
  cook_days: number
  warnings: number
}

/** The hard rules of spec §6.4. */
export type IssueRule = "H1" | "H2" | "H3" | "H4" | "H5" | "H6" | "H7" | "H8"

/** D22 codes; H3/H4 reuse the session warning codes. */
export type IssueCode =
  | "missing_plate"
  | "plate_in_none_cell"
  | "too_many_cook_occasions"
  | "diet_incompatible"
  | "meat_days_exceeded"
  | "diet_unknown"
  | "variety_repeat"
  | "variety_relaxed"
  | SessionWarningCode

/** A checker finding (D22). Message key `plan.issue.<code>` with `params`. */
export interface Issue {
  rule: IssueRule
  code: IssueCode
  severity: "violation" | "warning"
  date?: string
  slot_id?: number
  plate_id?: number
  session_id?: number
  params: Record<string, string | number>
}

/** The window's drafts (D26); `run_id`/`notes` from the newest run with
 *  drafts in the window. */
export interface PlanDraft {
  plates: number
  sessions: number
  run_id: number | null
  notes: string | null
  chooser: Chooser | null
}

export interface Plan {
  window: PlanWindow
  slots: PlanSlot[]
  cells: PlanCell[]
  /** Sessions cooking in the window or linked to a plate in it. */
  sessions: Session[]
  days: PlanDay[]
  targets: PlanTargets | null
  stats: PlanStats
  /** H1, H2, H5 and H8 findings on the window (D21). */
  issues: Issue[]
  /** null when the window holds no drafts. */
  draft: PlanDraft | null
  /** The generation running for this window, if any (D9). */
  running_run_id: number | null
}

/** GET /api/plan?start&today. Without `start` the server opens the anchored
 *  window. */
export function getPlan(start?: string): Promise<Plan> {
  const qs = new URLSearchParams()
  if (start) qs.set("start", start)
  qs.set("today", todayISO())
  return apiFetch(`/plan?${qs}`)
}

/** DELETE /api/plan?from&to: clears the range. */
export function clearPlan(from: string, to: string): Promise<RemovedResponse> {
  const qs = new URLSearchParams({ from, to })
  return apiFetch(`/plan?${qs}`, { method: "DELETE" })
}

/** POST /api/plan/restore: all or nothing; 409 if a cell is taken. */
export function restorePlan(
  snapshot: PlanSnapshot
): Promise<{ plate_ids: number[] }> {
  return apiFetch("/plan/restore", {
    method: "POST",
    body: JSON.stringify(snapshot),
  })
}

/** POST /api/plan/accept: confirms every draft row in the window closure
 *  (D4) and returns them for Undo. */
export function acceptPlan(from: string, to: string): Promise<StateChange> {
  return apiFetch("/plan/accept", {
    method: "POST",
    body: JSON.stringify({ from, to }),
  })
}

/** POST /api/plan/unaccept: Undo of accept, returns exactly those rows to
 *  draft. */
export function unacceptPlan(rows: StateChange): Promise<StateChange> {
  return apiFetch("/plan/unaccept", {
    method: "POST",
    body: JSON.stringify(rows),
  })
}

/** POST /api/plan/discard: deletes every draft row in the window closure;
 *  Undo is restorePlan. */
export function discardPlan(
  from: string,
  to: string
): Promise<RemovedResponse> {
  return apiFetch("/plan/discard", {
    method: "POST",
    body: JSON.stringify({ from, to }),
  })
}

// ── Mid-week re-plan (§3.2) ───────────────────────────────────────────

export interface ReplanPreview {
  none_cells: { date: string; slot_id: number }[]
  confirmed_affected: {
    plate_id: number
    date: string
    slot_id: number
    food_name: string
  }[]
  spare: {
    session_id: number
    food_name: string
    portions: number
    freezable: boolean
  }[]
}

export interface ReplanInput {
  from: string
  to: string
  unlock_confirmed: boolean
  spare: "freeze" | "keep"
  regenerate: boolean
  instruction?: string
}

export interface ReplanResponse extends RemovedResponse {
  /** The generation started for the remaining cells, if `regenerate`. */
  run_id: number | null
}

/** GET /api/plan/replan/preview?from&to&today. */
export function getReplanPreview(
  from: string,
  to: string
): Promise<ReplanPreview> {
  const qs = new URLSearchParams({ from, to, today: todayISO() })
  return apiFetch(`/plan/replan/preview?${qs}`)
}

/** POST /api/plan/replan: 409 `error.plan.confirmed_affected` when confirmed
 *  plates are affected and `unlock_confirmed` is false. */
export function replan(input: ReplanInput): Promise<ReplanResponse> {
  return apiFetch("/plan/replan", {
    method: "POST",
    body: JSON.stringify(input),
  })
}
