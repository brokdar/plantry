import { apiFetch } from "./client"
import type { Draft } from "./import"

export type FindState =
  | "searching"
  | "ready"
  | "needs_check"
  | "failed"
  | "in_library"
  | "discarded"
export type FindReason = "taste" | "ingredients" | "too_long" | "looks_wrong"
export type FindOrigin = "adapted_light" | "adapted_medium" | "inspired"

export interface FindSource {
  url: string
  site: string
  author?: string
  rating?: number
  votes: number
  base: boolean
}

export interface FindFlag {
  code: string
  value?: number
  typical?: number
  reason?: string
}

export interface FindChange {
  kind: string
  from?: string
  to?: string
  reason?: string
}

/** One pipeline step in Plantry's words: an i18n key under
 *  `discovery.find.steps` and its parameters. */
export interface FindStep {
  key: string
  params?: Record<string, string>
}

export interface Find {
  id: number
  state: FindState
  dish: string
  draft: Draft | null
  source_url: string
  sources: FindSource[] | null
  flags: FindFlag[] | null
  origin: FindOrigin | null
  changes: FindChange[] | null
  steps: FindStep[] | null
  /** KIT-2/3 questions this find raised: `equipment:<tool>` or `skill`. */
  questions: string[] | null
  food_id: number | null
}

/** POST /api/finds {dish}: 202 with the searching find; 409
 *  `error.chef.off` when the Chef is off. */
export function requestFind(dish: string): Promise<Find> {
  return apiFetch("/finds", { method: "POST", body: JSON.stringify({ dish }) })
}

export function getFind(id: number): Promise<Find> {
  return apiFetch(`/finds/${id}`)
}

/** POST /api/finds/{id}/keep {food_id}: the review saved the find. */
export function keepFind(id: number, foodId: number): Promise<Find> {
  return apiFetch(`/finds/${id}/keep`, {
    method: "POST",
    body: JSON.stringify({ food_id: foodId }),
  })
}

/** POST /api/finds/{id}/discard {reason, ingredients?}: "Not for me". */
export function discardFind(id: number, reason: FindReason): Promise<Find> {
  return apiFetch(`/finds/${id}/discard`, {
    method: "POST",
    body: JSON.stringify({ reason }),
  })
}
