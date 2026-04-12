import { createFileRoute } from "@tanstack/react-router"
import { IngredientList } from "@/components/ingredients/IngredientList"

export const Route = createFileRoute("/ingredients/")({
  component: IngredientsPage,
})

function IngredientsPage() {
  return <IngredientList />
}
