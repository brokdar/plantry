import { apiFetch } from "./client"

/** One thing the assistant did in a window (PRO-5). */
export interface AssistantActivity {
  kind: "find_placed"
  find_id: number
  food_id: number
  name: string
}

/** GET /api/plan/assistant: the list within the noise budget, and how many
 *  more there are. */
export interface AssistantDid {
  items: AssistantActivity[]
  more: number
}

/** POST /api/plan/opened: the plan page shows this window (P11). */
export function planOpened(start: string, today: string): Promise<void> {
  return apiFetch("/plan/opened", {
    method: "POST",
    body: JSON.stringify({ start, today }),
  })
}

/** GET /api/plan/assistant?start=. */
export function getAssistantDid(start: string): Promise<AssistantDid> {
  return apiFetch(`/plan/assistant?start=${encodeURIComponent(start)}`)
}

/** POST /api/plan/assistant/finds/{id}/undo. */
export function undoPlacedFind(id: number): Promise<void> {
  return apiFetch(`/plan/assistant/finds/${id}/undo`, { method: "POST" })
}
