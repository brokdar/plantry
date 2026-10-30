import { todayISO } from "@/lib/planner-window"

import type { AvailabilityMode, AvailabilitySource } from "./availability"
import { apiFetch } from "./client"
import type { Plate, PlanSnapshot, RemovedResponse } from "./plates"
import type { Session } from "./sessions"

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

export interface Plan {
  window: PlanWindow
  slots: PlanSlot[]
  cells: PlanCell[]
  /** Sessions cooking in the window or linked to a plate in it. */
  sessions: Session[]
  days: PlanDay[]
  targets: PlanTargets | null
  stats: PlanStats
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
