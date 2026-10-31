import { createFileRoute } from "@tanstack/react-router"

import { RecipeDetailPage } from "@/components/recipes/RecipeDetailPage"
import { MAX_PORTIONS } from "@/lib/portions"

export const Route = createFileRoute("/recipes/$id/")({
  // `portions` starts the stepper (e.g. coming back from cook mode); out of
  // range or malformed values fall back to the recipe's own portions.
  validateSearch: (search): { portions?: number } => {
    const n = Number(search.portions)
    return {
      portions:
        Number.isInteger(n) && n >= 1 && n <= MAX_PORTIONS ? n : undefined,
    }
  },
  component: RecipeDetailRoute,
})

function RecipeDetailRoute() {
  const { id } = Route.useParams()
  const { portions } = Route.useSearch()
  return <RecipeDetailPage id={id} portions={portions} />
}
