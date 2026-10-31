import { createFileRoute } from "@tanstack/react-router"

import { CookRecipePage } from "@/components/cook/CookPages"
import { MAX_PORTIONS } from "@/lib/portions"

/** Portions the scaling endpoint accepts: 0 < p ≤ MAX_PORTIONS. */
function parsePortions(value: unknown): number | undefined {
  const n = typeof value === "string" ? Number(value) : value
  return typeof n === "number" && n > 0 && n <= MAX_PORTIONS ? n : undefined
}

export const Route = createFileRoute("/recipes/$id/cook")({
  validateSearch: (search): { portions?: number } => ({
    portions: parsePortions(search.portions),
  }),
  component: CookRecipeRoute,
})

function CookRecipeRoute() {
  const { id } = Route.useParams()
  const { portions } = Route.useSearch()
  return <CookRecipePage id={Number(id)} portions={portions} />
}
