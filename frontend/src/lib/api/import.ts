import type { LookupCandidate } from "./lookup"
import { apiFetch } from "./client"
import type { EffectiveDietClass } from "./foods"

export interface DraftIngredient {
  raw_text: string
  amount: number
  /** "g", "ml", a count unit ("piece", "clove"), or "" when unparsed. */
  unit: string
  original_unit: string
  name: string
  note: string
  confidence: "parsed" | "approximate" | "unparsed"
  /** Estimated grams per count unit (backend `units.CountGrams`); null for
   *  mass, volume and unparsed lines. */
  portion_grams: number | null
  /** A count line nobody can estimate: the review asks for a weight. */
  needs_weight: boolean
  /** Code of a hint about the line, e.g. "suspicious_unit". */
  warning: string
  /** The line's diet class from its name, read-only on review (D30). */
  diet_class: EffectiveDietClass
  /** Other foods the line offers ("oder Butterschmalz"). */
  alternatives?: string[] | null
  /** The line has no amount ("Salz", "nach Geschmack"). */
  to_taste?: boolean
  /** The matching path's result (MATCH-2); null when not matched. */
  match?: LineMatch | null
}

/** One line's match: matched, closest (an immaterial seasoning) or
 *  unmatched with the reason and what was tried. */
export interface LineMatch {
  status: "matched" | "closest" | "unmatched"
  reason?:
    | "no_candidate"
    | "state_mismatch"
    | "implausible_kcal"
    | "no_grams"
    | "no_nutrition"
  tried: string[]
  candidates: LookupCandidate[]
  /** Index into candidates; -1 when unmatched. */
  pick: number
  grams: number | null
  portion_grams?: number
  name: string
}

export interface Draft {
  name: string
  description: string
  source_url: string
  image_url: string
  prep_minutes: number | null
  cook_minutes: number | null
  total_minutes: number | null
  reference_portions: number
  instructions: string[]
  ingredients: DraftIngredient[]
  tags: string[]
  language: "de" | "en" | "unknown"
  extract_method: "jsonld" | "llm"
  warnings: string[]
  /** kcal per serving from the page's JSON-LD, for comparison only. */
  source_kcal_per_serving: number | null
}

export interface ExtractRequest {
  url?: string
  html?: string
}

export function extractRecipe(req: ExtractRequest): Promise<{ draft: Draft }> {
  return apiFetch(`/import/extract`, {
    method: "POST",
    body: JSON.stringify(req),
  })
}

export interface ImportLookupResponse {
  results: LookupCandidate[]
  recommended_index: number
  /** Why an empty result may not be a real "not found". */
  unavailable_reason?: "not_configured" | "sources_unavailable"
}

export function lookupImportLine(
  query: string,
  lang: string
): Promise<ImportLookupResponse> {
  const q = new URLSearchParams({ query, lang })
  return apiFetch(`/import/lookup?${q}`)
}

export type Resolution = "existing" | "skip" | "new"

export interface ResolveChildInput {
  resolution: Resolution
  food_id: number
  amount: number
  unit: string
  /** Grams per count unit the user edited in the review; null keeps the
   *  leaf's portion or the default estimate. */
  portion_grams: number | null
}

export interface ResolveInstructionInput {
  step_number: number
  text: string
}

export interface ResolveRequest {
  name: string
  role: string
  reference_portions: number
  prep_minutes: number | null
  cook_minutes: number | null
  notes: string | null
  tags: string[]
  instructions: ResolveInstructionInput[]
  children: ResolveChildInput[]
}

export interface FinalizedFoodChild {
  child_id: number
  amount: number
  unit: string
  grams: number
  grams_source: string
  sort_order: number
}

export interface FinalizedFood {
  name: string
  kind: "composed"
  role: string
  reference_portions: number
  prep_minutes: number | null
  cook_minutes: number | null
  notes: string | null
  children: FinalizedFoodChild[]
  instructions: { step_number: number; text: string }[]
  tags: string[]
}

export function resolveImport(
  req: ResolveRequest
): Promise<{ food: FinalizedFood }> {
  return apiFetch(`/import/resolve`, {
    method: "POST",
    body: JSON.stringify(req),
  })
}
