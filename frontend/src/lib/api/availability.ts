import { apiFetch } from "./client"
import type { Plate } from "./plates"

/** What a slot expects: a planned plate, a routine (preset), or no plate. */
export type AvailabilityMode = "plan" | "routine" | "none"

/** Where a resolved cell's mode comes from. */
export type AvailabilitySource = "default" | "override" | "fallback"

/** One weekday × slot default. Weekday 0 = Monday … 6 = Sunday. */
export interface AvailabilityDefault {
  weekday: number
  slot_id: number
  mode: AvailabilityMode
  preset_id: number | null
}

export interface AvailabilityDefaultsResponse {
  items: AvailabilityDefault[]
}

/** One resolved date × slot cell. */
export interface AvailabilityCell {
  date: string // YYYY-MM-DD
  slot_id: number
  mode: AvailabilityMode
  source: AvailabilitySource
  preset_id: number | null
  preset_name: string | null
  note: string | null
}

export interface AvailabilityRangeResponse {
  items: AvailabilityCell[]
}

export interface SetAvailabilityOverrideInput {
  mode: AvailabilityMode
  preset_id?: number | null
  note?: string | null
  /** Required to mark a cell that holds a plate as `none` (else 409). */
  remove_plate?: boolean
}

export interface SetAvailabilityOverrideResponse {
  cell: AvailabilityCell
  /** The plate deleted by `remove_plate`, loaded before the delete. */
  removed_plate?: Plate
}

export function getAvailabilityDefaults(): Promise<AvailabilityDefaultsResponse> {
  return apiFetch("/availability/defaults")
}

export function setAvailabilityDefaults(
  items: AvailabilityDefault[]
): Promise<AvailabilityDefaultsResponse> {
  return apiFetch("/availability/defaults", {
    method: "PUT",
    body: JSON.stringify({ items }),
  })
}

export function getAvailabilityRange(
  from: string,
  to: string
): Promise<AvailabilityRangeResponse> {
  return apiFetch(
    `/availability?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`
  )
}

export function setAvailabilityOverride(
  date: string,
  slotId: number,
  input: SetAvailabilityOverrideInput
): Promise<SetAvailabilityOverrideResponse> {
  return apiFetch(`/availability/${encodeURIComponent(date)}/${slotId}`, {
    method: "PUT",
    body: JSON.stringify(input),
  })
}

export function clearAvailabilityOverride(
  date: string,
  slotId: number
): Promise<void> {
  return apiFetch(`/availability/${encodeURIComponent(date)}/${slotId}`, {
    method: "DELETE",
  })
}
