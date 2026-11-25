import { apiFetch } from "./client"

export type Skill = "beginner" | "home_cook" | "confident"
export const SKILLS: readonly Skill[] = ["beginner", "home_cook", "confident"]

export type Heat = "none" | "mild" | "medium" | "hot"
export const HEATS: readonly Heat[] = ["none", "mild", "medium", "hot"]

/** The checklist, in display order (KIT-3). */
export const EQUIPMENT_TOOLS = [
  "oven",
  "stovetop",
  "microwave",
  "air_fryer",
  "pressure_cooker",
  "slow_cooker",
  "blender",
  "food_processor",
  "stand_mixer",
  "wok",
  "grill",
] as const
export type EquipmentTool = (typeof EQUIPMENT_TOOLS)[number]

export const MAX_LEANINGS = 5
export const MAX_LEANING_LENGTH = 40

export interface KitchenFood {
  id: number
  name: string
}

/** The settings half of `/api/kitchen`; the PUT body is a full replace. */
export interface KitchenSettings {
  minutes_weekday: number
  minutes_weekend: number
  /** null while unknown; an unknown skill counts as home cook. */
  skill: Skill | null
  /** The tools present; one not listed counts as absent. */
  equipment: EquipmentTool[]
  /** null while unknown. */
  heat: Heat | null
  leanings: string[]
}

export interface Kitchen extends KitchenSettings {
  /** Foods always at home, by name. */
  basics: KitchenFood[]
}

export function getKitchen(): Promise<Kitchen> {
  return apiFetch("/kitchen")
}

export function updateKitchen(input: KitchenSettings): Promise<Kitchen> {
  return apiFetch("/kitchen", { method: "PUT", body: JSON.stringify(input) })
}

export function addBasic(foodId: number): Promise<void> {
  return apiFetch(`/kitchen/basics/${foodId}`, { method: "PUT" })
}

export function removeBasic(foodId: number): Promise<void> {
  return apiFetch(`/kitchen/basics/${foodId}`, { method: "DELETE" })
}
