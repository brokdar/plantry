import { apiFetch } from "./client"

/** This month's spend of capped Chef work against `chef.monthly_cap`. */
export interface ChefSpend {
  /** YYYY-MM */
  month: string
  spent_usd: number
  cap_usd: number
  at_cap: boolean
}

/** GET /api/chef/spend. */
export function getChefSpend(): Promise<ChefSpend> {
  return apiFetch("/chef/spend")
}

/** What the one-time library labelling would label and roughly cost. */
export interface LabellingEstimate {
  recipes: number
  calls: number
  /** null when the configured model has no price. */
  cost_usd: number | null
  minutes: number
}

/** GET /api/chef/labelling. */
export function getLabellingEstimate(): Promise<LabellingEstimate> {
  return apiFetch("/chef/labelling")
}

/** POST /api/chef/labelling: queues the library labelling (Chef on). */
export function startLibraryLabelling(): Promise<{ jobs: number }> {
  return apiFetch("/chef/labelling", { method: "POST" })
}

/** POST /api/chef/fill-library: ten requested finds that enter the library
 *  on trial (COLD-3). */
export function fillLibrary(): Promise<{ items: { id: number }[] }> {
  return apiFetch("/chef/fill-library", { method: "POST" })
}
