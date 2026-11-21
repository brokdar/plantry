import { apiFetch } from "./client"

/** What a slot's plates must hold: a full meal needs a main, a light meal
 *  (breakfast, snack) takes anything. */
export type SlotKind = "light" | "meal"

export interface TimeSlot {
  id: number
  name_key: string
  icon: string
  sort_order: number
  active: boolean
  kind: SlotKind
  /** The effective energy band, in percent of the day's energy. */
  band_min_pct?: number
  band_max_pct?: number
  /** True when the user set the band (else it is the default). */
  band_custom?: boolean
}

export interface TimeSlotInput {
  name_key: string
  icon: string
  sort_order: number
  active: boolean
  kind: SlotKind
}

export interface TimeSlotListResponse {
  items: TimeSlot[]
}

export function listTimeSlots(
  activeOnly = false
): Promise<TimeSlotListResponse> {
  const qs = activeOnly ? "?active=true" : ""
  return apiFetch(`/settings/slots${qs}`)
}

export function createTimeSlot(input: TimeSlotInput): Promise<TimeSlot> {
  return apiFetch("/settings/slots", {
    method: "POST",
    body: JSON.stringify(input),
  })
}

export function updateTimeSlot(
  id: number,
  input: TimeSlotInput
): Promise<TimeSlot> {
  return apiFetch(`/settings/slots/${id}`, {
    method: "PUT",
    body: JSON.stringify(input),
  })
}

export function deleteTimeSlot(id: number): Promise<void> {
  return apiFetch(`/settings/slots/${id}`, { method: "DELETE" })
}

export interface SlotBand {
  slot_id: number
  min_pct: number
  max_pct: number
}

export interface SlotBandsResponse {
  items: SlotBand[]
  /** Expected share of the day per slot id, in percent. */
  shares: Record<string, number>
}

export type SlotBandPreset = "lunch_biggest" | "dinner_biggest"

export function getSlotBands(): Promise<SlotBandsResponse> {
  return apiFetch("/settings/slots/bands")
}

export function setSlotBands(items: SlotBand[]): Promise<SlotBandsResponse> {
  return apiFetch("/settings/slots/bands", {
    method: "PUT",
    body: JSON.stringify({ items }),
  })
}

export function applySlotBandPreset(
  name: SlotBandPreset
): Promise<SlotBandsResponse> {
  return apiFetch("/settings/slots/bands/preset", {
    method: "POST",
    body: JSON.stringify({ name }),
  })
}
