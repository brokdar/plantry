import { apiFetch } from "./client"

export type PlateFeedbackStatus = "cooked" | "skipped" | "loved" | "disliked"

export interface PlateFeedback {
  plate_id: number
  status: PlateFeedbackStatus
  note?: string | null
  rated_at: string
}

export interface PutFeedbackInput {
  status: PlateFeedbackStatus
  note?: string | null
}

export function putFeedback(
  plateId: number,
  input: PutFeedbackInput
): Promise<PlateFeedback> {
  return apiFetch(`/plates/${plateId}/feedback`, {
    method: "PUT",
    body: JSON.stringify(input),
  })
}

export function deleteFeedback(plateId: number): Promise<void> {
  return apiFetch(`/plates/${plateId}/feedback`, { method: "DELETE" })
}
