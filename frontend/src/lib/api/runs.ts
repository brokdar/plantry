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
  | "chef_attributes"
  | "chef_match"
  | "chef_import_check"
  | "chef_find"
  | "chef_help"
  | "chef_library_review"

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
  /** Provider-reported reasoning tokens (inside output). */
  reasoning: number
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
  /** Web searches the run's model calls made (priced per search). */
  web_search_requests: number
  score: Score | null
  outcome: RunOutcome | null
  correction_rate: number | null
  /** Plan changes made by the agent in this run; null for non-chat runs. */
  edits: number | null
  /** Wall time in ms; null for runs older than the column or still running. */
  duration_ms: number | null
}

export interface RunStep {
  seq: number
  type: "llm_call" | "tool_call"
  name: string
  /** Raw JSON text, cut at the server's limit when `args_truncated`. */
  args: string | null
  args_truncated: boolean
  result: string | null
  result_truncated: boolean
  is_error: boolean
  duration_ms: number
  tokens: {
    input: number
    output: number
    cache_read: number
    cache_write: number
    reasoning: number
  }
  web_search_requests: number
  started_at: string | null
  /** Time to the first streamed token or tool call. */
  ttft_ms: number | null
  /** Thinking text length (Anthropic counts thinking inside output tokens). */
  thinking_chars: number
  /** Model the provider answered with (`name` is the requested one). */
  served_model: string | null
}

export interface RunDetail extends RunSummary {
  scope: unknown
  instruction: string | null
  notes: string | null
  violations: Issue[] | null
  error_key: string | null
  steps: RunStep[]
  prompt_text: string | null
}

export interface RunsPage {
  items: RunSummary[]
  /** The id to pass as `cursor` for the next page; null on the last. */
  next_cursor: number | null
}

export interface RunsParams {
  kind?: RunKind
  from?: string
  to?: string
  cursor?: number | null
  limit?: number
}

/** GET /api/ai/runs?kind&from&to&cursor&limit (default 50). */
export function listRuns(params: RunsParams = {}): Promise<RunsPage> {
  const qs = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== null) qs.set(key, String(value))
  }
  const query = qs.toString()
  return apiFetch(query ? `/ai/runs?${query}` : "/ai/runs")
}

/** GET /api/ai/runs/{id}: the summary plus steps and raw prompt. */
export function getRun(id: number): Promise<RunDetail> {
  return apiFetch(`/ai/runs/${id}`)
}
