import { todayISO } from "@/lib/planner-window"

import { apiFetch } from "./client"

/** The nine store sections in walk order (D18, UX §5.7). */
export const SHOP_SECTIONS = [
  "produce",
  "bakery",
  "dairy_eggs",
  "meat_fish",
  "dry_goods",
  "cans_jars",
  "spices_oils",
  "frozen",
  "other",
] as const

export type ShopSection = (typeof SHOP_SECTIONS)[number]

/** A rounded quantity from the backend (D26). */
export interface Quantity {
  amount: number
  unit: string
}

export interface ShoppingUsedIn {
  /** null for a leaf component on a plate. */
  session_id: number | null
  food_id: number
  food_name: string
  date: string
  slot_id: number
}

export interface ShoppingListItem {
  food_id: number
  name: string
  total_grams: number
  display: Quantity
  /** Count units only: what to buy (ceil). */
  buy: Quantity | null
  used_in: ShoppingUsedIn[]
}

export interface ShoppingSection {
  section: ShopSection
  items: ShoppingListItem[]
}

export interface ShoppingList {
  range: { from: string; to: string; window_start: string; window_end: string }
  sections: ShoppingSection[]
  count: number
}

export interface ShoppingRange {
  from: string
  to: string
}

/** GET /api/shopping-list?from&to&today. Without a range the server uses
 *  the default of the anchored window. */
export function getShoppingList(range?: ShoppingRange): Promise<ShoppingList> {
  const qs = new URLSearchParams()
  if (range) {
    qs.set("from", range.from)
    qs.set("to", range.to)
  }
  qs.set("today", todayISO())
  return apiFetch(`/shopping-list?${qs}`)
}

// Kept for the old planner's ShoppingPanel until F7 deletes it: the current
// backend still answers GET /api/shopping-list with `{items}` until B6.
export interface PlannerShoppingItem {
  food_id: number
  name: string
  total_grams: number
}

export function getPlannerShoppingList(
  from: string,
  to: string
): Promise<{ items: PlannerShoppingItem[] }> {
  return apiFetch(
    `/shopping-list?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`
  )
}
