import { createFileRoute } from "@tanstack/react-router"

import { RecipeDetailPage } from "@/components/components/RecipeDetailPage"

export const Route = createFileRoute("/components/$id/")({
  component: RecipeDetailRoute,
})

function RecipeDetailRoute() {
  const { id } = Route.useParams()
  return <RecipeDetailPage id={id} />
}
