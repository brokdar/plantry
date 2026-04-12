import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { IngredientEditor } from "@/components/ingredients/IngredientEditor"

export const Route = createFileRoute("/ingredients/new")({
  component: NewIngredientPage,
})

function NewIngredientPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()

  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-semibold tracking-tight">
        {t("ingredient.create")}
      </h1>
      <IngredientEditor onSuccess={() => navigate({ to: "/ingredients" })} />
    </section>
  )
}
