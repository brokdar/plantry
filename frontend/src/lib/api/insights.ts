import { apiFetch } from "./client"
import type { RunKind, RunTokens } from "./runs"

export interface InsightsWeek {
  start: string
  runs: number
  accepted: number
  discarded: number
  correction_rate: number | null
  cost_usd: number
  cost_per_accepted: number | null
  /** Runs left out of the cost because their model has no price. */
  unpriced_runs: number
}

export interface InsightsByVersion {
  kind: RunKind
  prompt_version: string
  model: string
  runs: number
  tokens: RunTokens
  /** Agent plan changes in the group; null when the kind has none. */
  edits: number | null
  /** Average of each numeric score component (D23). */
  score_avg: Record<string, number>
  /** Hard violations per rule ("H1" → n). */
  violations: Record<string, number>
}

export interface Insights {
  weeks: InsightsWeek[]
  by_version: InsightsByVersion[]
  prices_as_of: string
}

export interface InsightsParams {
  from?: string
  to?: string
}

/** GET /api/ai/insights?from&to&bucket=week. */
export function getInsights(params: InsightsParams = {}): Promise<Insights> {
  const qs = new URLSearchParams()
  if (params.from) qs.set("from", params.from)
  if (params.to) qs.set("to", params.to)
  qs.set("bucket", "week")
  return apiFetch(`/ai/insights?${qs}`)
}
