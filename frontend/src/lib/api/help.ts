import { apiFetch } from "./client"

export type HelpProblem =
  | "too_salty"
  | "too_thin"
  | "too_thick"
  | "burnt"
  | "not_cooking_through"

export const HELP_PROBLEMS: readonly HelpProblem[] = [
  "too_salty",
  "too_thin",
  "too_thick",
  "burnt",
  "not_cooking_through",
]

export interface HelpInput {
  session_id: number
  /** 0-based step on screen. */
  step_index: number
  kind: "explain_step" | "went_wrong"
  problem?: HelpProblem
  text?: string
}

/** POST /api/help — a model answer of at most three lines, amount first. */
export function askHelp(
  input: HelpInput
): Promise<{ answer: string[]; run_id: number }> {
  return apiFetch("/help", { method: "POST", body: JSON.stringify(input) })
}

export type HelpVerdict = "helped" | "not_helped"

/** PUT /api/chef/help/{runId}/verdict — the one-tap "Helped" / "Didn't help";
 *  a later call changes it. */
export function setHelpVerdict(
  runId: number,
  verdict: HelpVerdict
): Promise<void> {
  return apiFetch(`/chef/help/${runId}/verdict`, {
    method: "PUT",
    body: JSON.stringify({ verdict }),
  })
}
