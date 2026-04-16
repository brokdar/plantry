import { apiFetch } from "./client"

export interface TimeSlot {
  id: number
  name_key: string
  icon: string
  sort_order: number
  active: boolean
}

export interface TimeSlotInput {
  name_key: string
  icon: string
  sort_order: number
  active: boolean
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
