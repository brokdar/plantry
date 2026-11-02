import { apiFetch } from "./client"
import type { EffectiveDietClass } from "./foods"
import type { Draft } from "./import"

export type DiscoveryMode = "daily" | "explore" | "known"
export type SuggestionStatus = "pending" | "accepted" | "rejected"
export type RejectReason = "too_long" | "ingredients" | "taste"
export type DiscoveryRunStatus = "running" | "ok" | "error" | "partial"

export interface SuggestionIngredient {
  raw_text: string
  name: string
  amount: number | null
  unit: string | null
  food_id: number | null
  /** The matched leaf's name; null when unmatched. */
  candidate: string | null
  /** True when the ingredient matches a food the user already has. */
  known: boolean
}

export interface Suggestion {
  id: number
  mode: DiscoveryMode
  query: string | null
  source_url: string
  image_url: string | null
  /** Found through web search (shown as "Search-grounded"). */
  grounded: boolean
  status: SuggestionStatus
  reason: RejectReason | null
  run_id: number | null
  /** The saved food once accepted. */
  food_id: number | null
  shown_on: string | null
  created_at: string
  name: string
  cuisine: string | null
  total_minutes: number | null
  reference_portions: number
  diet_class: EffectiveDietClass
  per_portion: { kcal: number; protein: number; fat: number; carbs: number }
  nutrition_complete: boolean
  missing: string[]
  why: string
  known_share: number
  ingredients: SuggestionIngredient[]
  draft: Draft
}

export interface DiscoverInput {
  mode: DiscoveryMode
  /** Required for `explore`. */
  cuisine?: string
  focus?: string
  today?: string
}

export interface DiscoveryRun {
  run_id: number
  status: DiscoveryRunStatus
  suggestion_ids: number[]
  discarded: number
  error_key: string | null
}

/**
 * The daily endpoints answer 200 {suggestion}, 202 {run_id} (started or
 * running) or 204 (disabled, no provider, or the run failed: silent).
 */
export type DailyResult =
  | { kind: "suggestion"; suggestion: Suggestion }
  | { kind: "running"; runId: number }
  | { kind: "none" }

type DailyResponse = { suggestion: Suggestion } | { run_id: number } | undefined

function toDailyResult(res: DailyResponse): DailyResult {
  if (res && "suggestion" in res)
    return { kind: "suggestion", suggestion: res.suggestion }
  if (res && "run_id" in res) return { kind: "running", runId: res.run_id }
  return { kind: "none" }
}

export interface SuggestionsParams {
  status?: SuggestionStatus
  run_id?: number
}

/** POST /api/discovery: starts a run. */
export function discover(input: DiscoverInput): Promise<{ run_id: number }> {
  return apiFetch("/discovery", {
    method: "POST",
    body: JSON.stringify(input),
  })
}

/** GET /api/discovery/runs/{id}. */
export function getDiscoveryRun(id: number): Promise<DiscoveryRun> {
  return apiFetch(`/discovery/runs/${id}`)
}

/** GET /api/discovery/daily?date=. */
export async function getDailySuggestion(date: string): Promise<DailyResult> {
  const qs = new URLSearchParams({ date })
  return toDailyResult(await apiFetch<DailyResponse>(`/discovery/daily?${qs}`))
}

/** POST /api/discovery/daily/another {date}. */
export async function anotherSuggestion(date: string): Promise<DailyResult> {
  return toDailyResult(
    await apiFetch<DailyResponse>("/discovery/daily/another", {
      method: "POST",
      body: JSON.stringify({ date }),
    })
  )
}

/** GET /api/discovery/suggestions?status&run_id. */
export function listSuggestions(
  params: SuggestionsParams = {}
): Promise<{ items: Suggestion[] }> {
  const qs = new URLSearchParams()
  if (params.status) qs.set("status", params.status)
  if (params.run_id !== undefined) qs.set("run_id", String(params.run_id))
  const query = qs.toString()
  return apiFetch(
    query ? `/discovery/suggestions?${query}` : "/discovery/suggestions"
  )
}

/** POST /api/discovery/suggestions/{id}/accept {food_id}. */
export async function acceptSuggestion(
  id: number,
  foodId: number
): Promise<Suggestion> {
  const res = await apiFetch<{ suggestion: Suggestion }>(
    `/discovery/suggestions/${id}/accept`,
    { method: "POST", body: JSON.stringify({ food_id: foodId }) }
  )
  return res.suggestion
}

/** POST /api/discovery/suggestions/{id}/reject {reason?}. */
export async function rejectSuggestion(
  id: number,
  reason?: RejectReason
): Promise<Suggestion> {
  const res = await apiFetch<{ suggestion: Suggestion }>(
    `/discovery/suggestions/${id}/reject`,
    { method: "POST", body: JSON.stringify(reason ? { reason } : {}) }
  )
  return res.suggestion
}
