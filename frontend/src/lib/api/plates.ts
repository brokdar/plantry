import { apiFetch } from "./client"
import type { PlateFeedback } from "./feedback"

export interface MacrosResponse {
  kcal: number
  protein: number
  fat: number
  carbs: number
  fiber: number
  sodium: number
}

// Week is kept as an in-memory aggregate for plate-patch helpers used by
// optimistic component mutations (swap, add, remove). It is not fetched from
// the server; the week cache slots are populated by the mutation hooks only.
export interface Week {
  id: number
  year: number
  week_number: number
  plates: Plate[]
  created_at: string
}

export interface PlateComponent {
  id: number
  plate_id: number
  food_id: number
  /** Composed-food component: integer count of servings. */
  portions?: number
  /** Leaf-food component: user-entered quantity. */
  amount?: number
  /** Leaf-food component: canonical unit key (g, ml, piece…). */
  unit?: string
  /** Leaf-food component: server-resolved grams snapshot. */
  grams?: number
  /** Leaf-food component: confidence label for the grams resolution. */
  grams_source?: string
  sort_order: number
}

export interface Plate {
  id: number
  slot_id: number
  date: string
  note: string | null
  skipped: boolean
  components: PlateComponent[]
  feedback?: PlateFeedback | null
  created_at: string
}

export interface UpdatePlateInput {
  slot_id?: number
  note?: string | null
  date?: string
}

export interface AddPlateComponentInput {
  food_id: number
  portions: number
}

export interface UpdatePlateComponentInput {
  food_id?: number
  portions?: number
}

export function listPlates(
  from: string,
  to: string
): Promise<{ plates: Plate[] }> {
  return apiFetch(
    `/plates?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`
  )
}

export function createPlate(input: {
  date: string
  slot_id: number
  note?: string
}): Promise<Plate> {
  return apiFetch("/plates", { method: "POST", body: JSON.stringify(input) })
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

export interface PlateMacrosEntry {
  plate_id: number
  date: string
  skipped: boolean
  macros: MacrosResponse
}

/** GET /api/plates/macros?from=&to= — per-plate macro totals for a date range. */
export function listPlateMacros(
  from: string,
  to: string
): Promise<{ plates: PlateMacrosEntry[] }> {
  return apiFetch(
    `/plates/macros?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`
  )
}
