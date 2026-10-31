import { apiFetch } from "./client"

/** USD per 1M tokens. */
export interface ModelPrice {
  in: number
  out: number
  /** null when the model has no cache price. */
  cache_read: number | null
  cache_write: number | null
}

export interface PriceInfo {
  model: string
  price: ModelPrice | null
  source: "builtin" | "override" | "none"
  as_of: string
}

/** GET /api/ai/prices?model=. */
export function getPrice(model: string): Promise<PriceInfo> {
  const qs = new URLSearchParams({ model })
  return apiFetch(`/ai/prices?${qs}`)
}
