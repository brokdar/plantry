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
