import { createFileRoute } from "@tanstack/react-router"

import { RecipeLibrary } from "@/components/recipes/RecipeLibrary"
import { parseLibrarySearch, type LibrarySearch } from "@/lib/recipes/library"

export const Route = createFileRoute("/recipes/")({
  validateSearch: (search): LibrarySearch => parseLibrarySearch(search),
  component: RecipesPage,
})

function RecipesPage() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <RecipeLibrary
      search={search}
      onSearchChange={(next, replace) =>
        void navigate({ search: next, replace })
      }
    />
  )
}
