import { apiFetch } from "./client"
import type { Issue } from "./plan"

/** Who picks the recipes (D11): the model, or the deterministic heuristic. */
export type Chooser = "llm" | "heuristic"

export interface GenerateInput {
  /** A window start (`error.plan.invalid_start` otherwise). */
  start: string
  chooser?: Chooser
  instruction?: string
  /** Guests: eaters per date (D16). */
  eaters_by_date?: Record<string, number>
  /** "Include a new recipe": one never eaten (D15). */
  include_new_recipe?: boolean
}

/** What a regenerate may replace (D6). The `window` scope is generate (D7). */
export type RegenerateScope =
  | { kind: "day"; date: string }
  | { kind: "plate"; plate_id: number }
  | { kind: "side"; component_id: number }
  | { kind: "empty_cells" }

export interface RegenerateInput {
  scope: RegenerateScope
  start: string
  chooser?: Chooser
  instruction?: string
}

export interface RunStarted {
  run_id: number
}

export type PlanRunStatus = "running" | "ok" | "error" | "partial"

/** Generation stages, in order (D35). */
export type RunStage =
  | "resolving"
  | "choosing"
  | "checking"
  | "repairing"
  | "saving"

/** Score components (D23); no weighted total. */
export interface Score {
  coverage: number
  hard_violations: Record<string, number>
  macro_deviation: { daily_pct: number; weekly_pct: number }
  cook_occasions: { per_day: number; days_with_prepared: number }
  variety: { distinct: number; repeats: number; new_recipes: number }
  preference_fit: number
  waste_portions: number
  unknown_data_share: number
}

export interface PlanRun {
  id: number
  kind: "generate" | "regenerate"
  status: PlanRunStatus
  stage: RunStage | null
  chooser: Chooser | null
  notes: string | null
  error_key: string | null
  counts: {
    plates: number
    sessions: number
    cook_days: number
    kept_confirmed: number
  }
  issues: Issue[]
  score: Score | null
  window: { start: string; end: string }
}

/** POST /api/plan/generate → 202 {run_id}; 409
 *  `error.plan.generation_running` while one runs for the window. */
export function generatePlan(input: GenerateInput): Promise<RunStarted> {
  return apiFetch("/plan/generate", {
    method: "POST",
    body: JSON.stringify(input),
  })
}

/** POST /api/plan/regenerate → 202 {run_id}; 409
 *  `error.plan.scope_confirmed` for a confirmed plate or side. */
export function regeneratePlan(input: RegenerateInput): Promise<RunStarted> {
  return apiFetch("/plan/regenerate", {
    method: "POST",
    body: JSON.stringify(input),
  })
}

/** GET /api/plan/runs/{id}: progress and result of a generation (D9). */
export function getPlanRun(id: number): Promise<PlanRun> {
  return apiFetch(`/plan/runs/${id}`)
}
