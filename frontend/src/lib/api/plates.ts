import { apiFetch } from "./client"
import type { PlateFeedback } from "./feedback"

export interface PlateComponent {
  id: number
  plate_id: number
  food_id: number
  portions: number
  sort_order: number
}

export interface Plate {
  id: number
  week_id: number
  day: number
  slot_id: number
  note: string | null
  skipped: boolean
  components: PlateComponent[]
  feedback?: PlateFeedback | null
  created_at: string
}

export interface UpdatePlateInput {
  day?: number
  slot_id?: number
  note?: string | null
}

export interface AddPlateComponentInput {
  food_id: number
  portions: number
}

export interface UpdatePlateComponentInput {
  food_id?: number
  portions?: number
}

export function getPlate(id: number): Promise<Plate> {
  return apiFetch(`/plates/${id}`)
}

export function updatePlate(
  id: number,
  input: UpdatePlateInput
): Promise<Plate> {
  return apiFetch(`/plates/${id}`, {
    method: "PUT",
    body: JSON.stringify(input),
  })
}

export function deletePlate(id: number): Promise<void> {
  return apiFetch(`/plates/${id}`, { method: "DELETE" })
}

export interface SetPlateSkippedInput {
  skipped: boolean
  note?: string | null
}

export function setPlateSkipped(
  id: number,
  input: SetPlateSkippedInput
): Promise<Plate> {
  return apiFetch(`/plates/${id}/skip`, {
    method: "POST",
    body: JSON.stringify(input),
  })
}

export function addPlateComponent(
  plateId: number,
  input: AddPlateComponentInput
): Promise<PlateComponent> {
  return apiFetch(`/plates/${plateId}/components`, {
    method: "POST",
    body: JSON.stringify(input),
  })
}

export function updatePlateComponent(
  plateId: number,
  pcId: number,
  input: UpdatePlateComponentInput
): Promise<PlateComponent> {
  return apiFetch(`/plates/${plateId}/components/${pcId}`, {
    method: "PUT",
    body: JSON.stringify(input),
  })
}

export function deletePlateComponent(
  plateId: number,
  pcId: number
): Promise<void> {
  return apiFetch(`/plates/${plateId}/components/${pcId}`, { method: "DELETE" })
}
