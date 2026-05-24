import { apiFetch } from "./client"

export type UnitGroup = "mass" | "volume" | "count"

export interface UnitDescriptor {
  id: string
  group: UnitGroup
  grams?: number
  approximate?: boolean
}

export function getUnits(): Promise<UnitDescriptor[]> {
  return apiFetch("/units")
}
