import { createFileRoute } from "@tanstack/react-router"
import { useEffect } from "react"

import { RecipeLibrary } from "@/components/recipes/RecipeLibrary"
import {
  parseLibrarySearch,
  rememberLibrarySearch,
  type LibrarySearch,
} from "@/lib/recipes/library"

export const Route = createFileRoute("/recipes/")({
  validateSearch: (search): LibrarySearch => parseLibrarySearch(search),
  component: RecipesPage,
})

function RecipesPage() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  useEffect(() => rememberLibrarySearch(search), [search])
  return (
    <RecipeLibrary
      search={search}
      onSearchChange={(next, replace) =>
        void navigate({ search: next, replace })
      }
    />
  )
}
