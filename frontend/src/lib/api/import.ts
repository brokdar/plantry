import type { LookupCandidate } from "./lookup"
import { apiFetch } from "./client"

export interface DraftIngredient {
  raw_text: string
  amount: number
  unit: string
  original_unit: string
  name: string
  note: string
  confidence: "parsed" | "approximate" | "unparsed"
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
}

export interface ExtractRequest {
  url?: string
  html?: string
}

export function extractRecipe(req: ExtractRequest): Promise<{ draft: Draft }> {
  return apiFetch("/import/extract", {
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

export interface ResolveIngredientInput {
  resolution: Resolution
  existing_ingredient_id?: number
  amount: number
  unit: string
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
  ingredients: ResolveIngredientInput[]
}

export interface ResolvedComponent {
  name: string
  role: string
  reference_portions: number
  prep_minutes: number | null
  cook_minutes: number | null
  notes: string | null
  ingredients: {
    ingredient_id: number
    amount: number
    unit: string
    grams: number
    sort_order: number
  }[]
  instructions: { step_number: number; text: string }[]
  tags: string[]
}

export function resolveImport(
  req: ResolveRequest
): Promise<{ component: ResolvedComponent }> {
  return apiFetch("/import/resolve", {
    method: "POST",
    body: JSON.stringify(req),
  })
}
