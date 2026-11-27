import type { FoodEventUndo } from "./foods"
import { apiFetch } from "./client"

export type TasteLevel = "hero" | "cuisine" | "flavour" | "technique" | "format"
export type TastePolarity = "love" | "rather_not"
export type TasteSource = "learned" | "statement" | "user"

export interface TasteItem {
  id: number
  level: TasteLevel
  key: string
  scope: "always" | "window"
  window_start?: string
  source: TasteSource
  polarity: TastePolarity
  score: number
}

export interface LearnedItem {
  item: TasteItem
  at: string
  undo: FoodEventUndo
}

export interface TasteFood {
  id: number
  name: string
}

/** GET /api/taste: what the household loves and would rather not have. */
export interface Taste {
  items: TasteItem[]
  recent: LearnedItem[]
  /** The foods never eaten (the kitchen's never items). */
  never: TasteFood[]
  /** Recipes the household made its own. */
  staples: TasteFood[]
}

export function getTaste(window?: string): Promise<Taste> {
  return apiFetch(`/taste${window ? `?window=${window}` : ""}`)
}

/** Flips an item: the user's own polarity outranks what was learned. */
export function setItemPolarity(
  id: number,
  polarity: TastePolarity
): Promise<{ undo: FoodEventUndo }> {
  return apiFetch(`/taste/items/${id}`, {
    method: "PUT",
    body: JSON.stringify({ polarity }),
  })
}

export function removeItem(id: number): Promise<{ undo: FoodEventUndo }> {
  return apiFetch(`/taste/items/${id}`, { method: "DELETE" })
}

/** Makes the item's food a never item; 404 when no food has that name. */
export function makeItemNever(id: number): Promise<void> {
  return apiFetch(`/taste/items/${id}/never`, { method: "POST" })
}

export function removeNever(foodId: number): Promise<void> {
  return apiFetch(`/taste/never/${foodId}`, { method: "DELETE" })
}

/** What a record_preference result proposes (the chip's data). */
export type Statement =
  | {
      kind: "taste"
      level: TasteLevel
      key: string
      polarity: TastePolarity
      window_start: string
    }
  | { kind: "health"; word: string }

/** The user's tap on a statement chip. */
export function confirmStatement(
  statement: Statement,
  scope: "always" | "window"
): Promise<{ undo?: FoodEventUndo } | null> {
  return apiFetch(`/taste/statements/confirm`, {
    method: "POST",
    body: JSON.stringify({ ...statement, scope }),
  })
}
