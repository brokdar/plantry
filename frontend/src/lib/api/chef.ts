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

/** One recipe site the finds search (FIND-7). */
export interface ChefSite {
  domain: string
  lang: "en" | "de"
  enabled: boolean
  /** A built-in site: it can be disabled, not removed. */
  default: boolean
  /** The search provider refuses the site until then (ISO time); null
   *  otherwise. */
  blocked_until: string | null
}

/** GET /api/chef/sites. */
export function listChefSites(): Promise<{ items: ChefSite[] }> {
  return apiFetch("/chef/sites")
}

/** POST /api/chef/sites: adds a site, enabled; the domain is normalised. */
export function addChefSite(input: {
  domain: string
  lang: "en" | "de"
}): Promise<{ items: ChefSite[] }> {
  return apiFetch("/chef/sites", {
    method: "POST",
    body: JSON.stringify(input),
  })
}

const sitePath = (domain: string) => `/chef/sites/${encodeURIComponent(domain)}`

/** PUT /api/chef/sites/{domain}: switches a site on or off. */
export function setChefSiteEnabled(
  domain: string,
  enabled: boolean
): Promise<{ items: ChefSite[] }> {
  return apiFetch(sitePath(domain), {
    method: "PUT",
    body: JSON.stringify({ enabled }),
  })
}

/** DELETE /api/chef/sites/{domain}: a site the user added (409 for a
 *  default one). */
export function removeChefSite(domain: string): Promise<{ items: ChefSite[] }> {
  return apiFetch(sitePath(domain), { method: "DELETE" })
}

/** POST /api/chef/sites/{domain}/retry: clears the search block. */
export function retryChefSite(domain: string): Promise<{ items: ChefSite[] }> {
  return apiFetch(`${sitePath(domain)}/retry`, { method: "POST" })
}
