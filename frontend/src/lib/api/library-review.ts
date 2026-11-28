import { apiFetch } from "./client"

/** A recipe a finding names; `missing` lists the data it lacks. */
export interface ReviewRecipe {
  id: number
  name: string
  missing?: ("times" | "steps")[]
}

/** One thing the monthly library review noticed (LIB-2). */
export interface ReviewFinding {
  key: string
  kind: "duplicates" | "replaced" | "data"
  recipes: ReviewRecipe[]
  /** replaced: times taken off a plan; data: recipes lacking data. */
  count?: number
}

export interface LibraryReview {
  /** YYYY-MM */
  month: string
  findings: ReviewFinding[]
}

/** GET /api/library-review: this month's card, null when the review is off,
 *  dismissed or has nothing to say. */
export function getLibraryReview(): Promise<{ card: LibraryReview | null }> {
  return apiFetch("/library-review")
}

/** POST /api/library-review/dismiss: one finding, or the card without a key. */
export function dismissLibraryReview(key?: string): Promise<void> {
  return apiFetch("/library-review/dismiss", {
    method: "POST",
    body: JSON.stringify({ key: key ?? "" }),
  })
}

/** POST /api/library-review/variants: groups a duplicates finding's recipes
 *  as variants. */
export function makeReviewVariants(key: string): Promise<void> {
  return apiFetch("/library-review/variants", {
    method: "POST",
    body: JSON.stringify({ key }),
  })
}
