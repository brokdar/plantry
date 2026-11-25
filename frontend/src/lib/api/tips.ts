import { apiFetch } from "./client"
import type { Undo } from "./meal"

/** A tip's kind (TIP-2). */
export type TipKind =
  | "finishing_acid"
  | "fresh_herbs"
  | "crunch"
  | "quick_pickle"
  | "cool_element"
  | "sauce"
  | "side_swap"
  | "cuisine_fix"

/** One live tip: one sentence and an Apply holding a checked change. */
export interface Tip {
  id: number
  plate_id: number | null
  kind: TipKind
  dimension: string
  state: "shown" | "applied" | "dismissed" | "withdrawn"
  action: "add" | "swap"
  food_id: number
  food_name: string
  /** The side a swap takes off. */
  from_name?: string
  grams?: number
}

/** A plate's plate-fit reading: the raised dimensions (empty: it fits). */
export interface PlateFit {
  plate_id: number
  raised: string[]
}

/** GET /api/tips: `on` is false while the Chef is off (no tips, no fit). */
export interface TipsView {
  on: boolean
  start?: string
  tips: Tip[]
  fit: PlateFit[]
}

/** GET /api/tips?date=: the live tips of the window that holds `date`. */
export function getTips(date: string): Promise<TipsView> {
  return apiFetch(`/tips?date=${encodeURIComponent(date)}`)
}

/** POST /api/tips/{id}/apply: the change on the plate and its revert token. */
export function applyTip(id: number): Promise<{ undo: Undo }> {
  return apiFetch(`/tips/${id}/apply`, { method: "POST" })
}

/** POST /api/tips/{id}/dismiss. */
export function dismissTip(id: number): Promise<void> {
  return apiFetch(`/tips/${id}/dismiss`, { method: "POST" })
}

/** The Chef block of Insights (§10, first metrics). */
export interface ChefInsights {
  from: string
  to: string
  tips: { shown: number; applied: number; dismissed: number }
  /** YYYY-MM */
  month: string
  spend: { kind: string; usd: number }[]
}

/** GET /api/chef/insights. */
export function getChefInsights(): Promise<ChefInsights> {
  return apiFetch("/chef/insights")
}
