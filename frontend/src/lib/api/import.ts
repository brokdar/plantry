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
  /** A seasoning line without an amount (MATCH-6): saved "to taste". */
  to_taste?: boolean
  /** Amount and unit are the import's proposal for a quantity-less line. */
  proposed?: boolean
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
  /** Set only by the import check (IMP-2). */
  role?: string
  keep_days?: number
  freezable?: boolean
}

export type ImportEditField =
  | "prep_minutes"
  | "cook_minutes"
  | "keep_days"
  | "freezable"
  | "role"
  | "name"
  | "description"
  | "step"
  | "ingredient"

/** One field the import check changed; Undo puts `before` back. `index` is
 *  the step or ingredient, -1 otherwise. */
export interface ImportEdit {
  field: ImportEditField
  index: number
  before: unknown
  after: unknown
}

/** An ingredient edit's before and after. */
export interface IngredientEditValue {
  amount: number
  unit: string
  proposed: boolean
}

export type ImportChangeKind =
  | "time"
  | "unit"
  | "keep"
  | "safety_cue"
  | "clarity"
  | "lead_time"
  | "role"
  | "amount"
  | "translate"

export interface ImportChange {
  id: string
  kind: ImportChangeKind
  reason: string
  edits: ImportEdit[]
}

/** Something to look at; never changed (IMP-3). */
export interface ImportFlag {
  ingredient: number
  step: number
  text: string
}

/** The import check (IMP): its changes are already in the draft. `ran` is
 *  false with the Chef off or no key. */
export interface ImportCheck {
  ran: boolean
  changes: ImportChange[]
  flags: ImportFlag[]
  finishing_touch?: string
  cap_warning?: boolean
  /** The chef_import_check run; sent back with the outcome on Save. */
  run_id?: number
}

export interface ImportCheckOutcome {
  run_id: number
  food_id: number
  /** Kinds of the applied changes the user kept / put back. */
  kept: string[]
  undone: string[]
  touch: "none" | "added" | "skipped"
  flags: number
}

/** What the user did with the check's findings; sent after Save. */
export function postImportCheckOutcome(o: ImportCheckOutcome): Promise<void> {
  return apiFetch(`/import/check/outcome`, {
    method: "POST",
    body: JSON.stringify(o),
  })
}

export interface ExtractRequest {
  url?: string
  html?: string
  /** The user's language; the import check translates into it. */
  lang?: string
}

export interface ExtractResponse {
  draft: Draft
  check?: ImportCheck
}

export function extractRecipe(req: ExtractRequest): Promise<ExtractResponse> {
  return apiFetch(`/import/extract`, {
    method: "POST",
    body: JSON.stringify(req),
  })
}

/** The import check over a draft the client holds (a Discover add). */
export function checkImportDraft(
  draft: Draft,
  lang: string
): Promise<ExtractResponse> {
  return apiFetch(`/import/check`, {
    method: "POST",
    body: JSON.stringify({ draft, lang }),
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

export type Resolution = "existing" | "new"

export interface ResolveChildInput {
  resolution: Resolution
  food_id: number
  amount: number
  unit: string
  /** A to-taste line: amount and unit are ignored. */
  to_taste?: boolean
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
