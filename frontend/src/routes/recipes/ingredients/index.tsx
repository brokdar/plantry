import { createFileRoute } from "@tanstack/react-router"

import { IngredientTable } from "@/components/ingredients/IngredientTable"

export const Route = createFileRoute("/recipes/ingredients/")({
  component: IngredientTable,
})
