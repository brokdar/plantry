import { createFileRoute } from "@tanstack/react-router"

import { RecipeDetailPage } from "@/components/recipes/RecipeDetailPage"

export const Route = createFileRoute("/recipes/$id/")({
  component: RecipeDetailRoute,
})

function RecipeDetailRoute() {
  const { id } = Route.useParams()
  return <RecipeDetailPage id={id} />
}
