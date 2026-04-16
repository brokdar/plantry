import { apiFetch } from "./client"
import type { Plate } from "./plates"

export interface Week {
  id: number
  year: number
  week_number: number
  plates: Plate[]
  created_at: string
}

export interface WeekListResponse {
  items: Week[]
  total: number
}

export interface CreatePlateInput {
  day: number
  slot_id: number
  note?: string | null
  components?: { component_id: number; portions: number }[]
}

export interface CopyWeekInput {
  target_year: number
  target_week: number
}

export function getCurrentWeek(): Promise<Week> {
  return apiFetch(`/weeks/current`)
}

export function getWeekByDate(year: number, week: number): Promise<Week> {
  return apiFetch(`/weeks/by-date?year=${year}&week=${week}`)
}

export function getWeek(id: number): Promise<Week> {
  return apiFetch(`/weeks/${id}`)
}

export function listWeeks(limit = 25, offset = 0): Promise<WeekListResponse> {
  return apiFetch(`/weeks?limit=${limit}&offset=${offset}`)
}

export function copyWeek(id: number, input: CopyWeekInput): Promise<Week> {
  return apiFetch(`/weeks/${id}/copy`, {
    method: "POST",
    body: JSON.stringify(input),
  })
}

export function createPlate(
  weekId: number,
  input: CreatePlateInput
): Promise<Plate> {
  return apiFetch(`/weeks/${weekId}/plates`, {
    method: "POST",
    body: JSON.stringify(input),
  })
}

export interface ShoppingItem {
  ingredient_id: number
  name: string
  total_grams: number
}

export interface ShoppingListResponse {
  items: ShoppingItem[]
}

export interface MacrosResponse {
  kcal: number
  protein: number
  fat: number
  carbs: number
  fiber: number
  sodium: number
}

export interface NutritionDayResponse {
  day: number
  macros: MacrosResponse
}

export interface WeekNutritionResponse {
  days: NutritionDayResponse[]
  week: MacrosResponse
}

export function getShoppingList(id: number): Promise<ShoppingListResponse> {
  return apiFetch(`/weeks/${id}/shopping-list`)
}

export function getWeekNutrition(id: number): Promise<WeekNutritionResponse> {
  return apiFetch(`/weeks/${id}/nutrition`)
}
