import { apiFetch } from "./client"

export interface PlateComponent {
  id: number
  plate_id: number
  component_id: number
  portions: number
  sort_order: number
}

export interface Plate {
  id: number
  week_id: number
  day: number
  slot_id: number
  note: string | null
  components: PlateComponent[]
  created_at: string
}

export interface UpdatePlateInput {
  day?: number
  slot_id?: number
  note?: string | null
}

export interface AddPlateComponentInput {
  component_id: number
  portions: number
}

export interface UpdatePlateComponentInput {
  component_id?: number
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
