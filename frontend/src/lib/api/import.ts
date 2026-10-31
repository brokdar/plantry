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
  /** The line's diet class from its name, read-only on review (D30). */
  diet_class: EffectiveDietClass
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
