import { apiFetch } from "./client"
import type { Chooser, Score } from "./generation"
import type { Issue } from "./plan"

export type RunKind =
  | "generate"
  | "regenerate"
  | "chat"
  | "discovery"
  | "classify"
  | "import_fallback"
  | "lookup"
  | "untracked"

export type RunStatus =
  | "running"
  | "ok"
  | "error"
  | "iterations_exceeded"
  | "cancelled"
  | "partial"

/** Accept/discard state machine per run (D25). */
export type RunOutcome =
  | "open"
  | "accepted"
  | "discarded"
  | "partially_accepted"

export interface RunTokens {
  input: number
  output: number
  cache_read: number
  cache_write: number
}

export interface RunSummary {
  id: number
  kind: RunKind
  started_at: string
  finished_at: string | null
  status: RunStatus
  provider: string | null
  model: string | null
  prompt_version: string | null
  chooser: Chooser | null
  tokens: RunTokens
  /** null = the model has no price ("cost unknown"). */
  cost_estimate_usd: number | null
  score: Score | null
  outcome: RunOutcome | null
  correction_rate: number | null
}

export interface RunStep {
  seq: number
  type: "llm_call" | "tool_call"
  name: string
  args: unknown
  result: unknown
  is_error: boolean
  duration_ms: number
  tokens: RunTokens | null
}

export interface RunDetail extends RunSummary {
  scope: unknown
  instruction: string | null
  notes: string | null
  violations: Issue[]
  error_key: string | null
  steps: RunStep[]
  prompt_text: string | null
}

export interface RunsPage {
  items: RunSummary[]
  next_cursor: string | null
}

export interface RunsParams {
  kind?: RunKind
  from?: string
  to?: string
  cursor?: string
  limit?: number
}

/** GET /api/ai/runs?kind&from&to&cursor&limit (default 50). */
export function listRuns(params: RunsParams = {}): Promise<RunsPage> {
  const qs = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined) qs.set(key, String(value))
  }
  const query = qs.toString()
  return apiFetch(query ? `/ai/runs?${query}` : "/ai/runs")
}

/** GET /api/ai/runs/{id}: the summary plus steps and raw prompt. */
export function getRun(id: number): Promise<RunDetail> {
  return apiFetch(`/ai/runs/${id}`)
}
