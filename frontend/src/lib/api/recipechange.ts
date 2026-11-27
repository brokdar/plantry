import { apiFetch } from "./client"
import type { FoodEventUndo } from "./foods"

/** One gate finding (a block stops a save, a flag only warns). */
export interface GateFinding {
  code: string
  ingredient: number
  step: number
  params?: Record<string, string>
  /** English explanation; the UI prefers a translated line per code. */
  reason: string
}

export interface CheckLine {
  text: string
  matched: boolean
  food_id?: number
  food_name?: string
  reason?: string
}

export interface RecipeCheck {
  lines: CheckLine[]
  blocks: GateFinding[]
  flags: GateFinding[]
}

export interface RecipeProposal {
  name: string
  ingredients: string[]
  steps: string[]
  changes: string[]
}

export interface RecipeContent {
  name: string
  ingredients: string[]
  steps: string[]
}

/** A saved change: `food_id` is 0 when the check blocked it. */
export interface RecipeChangeResult {
  food_id: number
  changes: string[]
  undo: FoodEventUndo
  check: RecipeCheck
}

export interface EditPreview {
  before: RecipeContent
  after: RecipeContent
  /** Dates of confirmed plates from today on that hold the recipe. */
  confirmed_dates: string[]
  check: RecipeCheck
}

export interface AskResult {
  kind: "variant" | "edit" | "answer"
  proposal?: RecipeProposal
  variant?: RecipeChangeResult
  preview?: EditPreview
  answer?: string
}

/** POST /api/foods/{id}/ask — a variant is saved at once, an edit comes back
 *  as a preview, a question gets its answer. */
export function askAboutRecipe(
  id: number,
  request: string
): Promise<AskResult> {
  return apiFetch(`/foods/${id}/ask`, {
    method: "POST",
    body: JSON.stringify({ request }),
  })
}

/** POST /api/foods/{id}/edit — saves the edit the user confirmed. */
export function saveRecipeEdit(
  id: number,
  proposal: RecipeProposal
): Promise<RecipeChangeResult> {
  return apiFetch(`/foods/${id}/edit`, {
    method: "POST",
    body: JSON.stringify(proposal),
  })
}

/** GET /api/foods/{id}/check — gates and line matches, no model call. */
export function checkRecipe(id: number): Promise<RecipeCheck> {
  return apiFetch(`/foods/${id}/check`)
}

/** GET /api/foods/{id}/changes — the recipe's changes list. */
export function getRecipeChanges(id: number): Promise<{ items: string[] }> {
  return apiFetch(`/foods/${id}/changes`)
}
