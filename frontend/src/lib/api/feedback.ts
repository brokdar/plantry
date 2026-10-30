import { apiFetch } from "./client"

/** D17: "Didn't eat it" is `not_eaten`. */
export type PlateFeedbackStatus = "loved" | "disliked" | "not_eaten"

/** Feedback as embedded in Plate JSON (`plate.feedback`). */
export interface PlateFeedback {
  status: PlateFeedbackStatus
  note: string | null
  rated_at: string
}

/** PUT /api/plates/{id}/feedback response. */
export interface FeedbackRecord {
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
): Promise<FeedbackRecord> {
  return apiFetch(`/plates/${plateId}/feedback`, {
    method: "PUT",
    body: JSON.stringify(input),
  })
}

export function deleteFeedback(plateId: number): Promise<void> {
  return apiFetch(`/plates/${plateId}/feedback`, { method: "DELETE" })
}
