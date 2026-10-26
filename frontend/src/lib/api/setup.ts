import { apiFetch } from "./client"

/** Plan decision D14: one shape serves the `/` redirect, the guided Recipes
 *  state and the Generate gate. */
export interface SetupStatus {
  completed: boolean
  /** Number of composed foods (recipes). */
  recipe_count: number
  /** Recipes needed before planning (10). */
  recipe_target: number
}

export function getSetupStatus(): Promise<SetupStatus> {
  return apiFetch("/setup")
}

export function completeSetup(): Promise<SetupStatus> {
  return apiFetch("/setup/complete", { method: "POST" })
}
