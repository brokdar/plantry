import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { PageHeader } from "@/components/editorial/PageHeader"
import { IngredientEditor } from "@/components/ingredients/IngredientEditor"

export const Route = createFileRoute("/ingredients/new")({
  component: NewIngredientPage,
})

function NewIngredientPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()

  return (
    <div className="mx-auto max-w-6xl space-y-8 px-4 py-8 md:px-8 md:py-12">
      <PageHeader title={t("ingredient.create")} />
      <IngredientEditor onSuccess={() => navigate({ to: "/ingredients" })} />
    </div>
  )
}
