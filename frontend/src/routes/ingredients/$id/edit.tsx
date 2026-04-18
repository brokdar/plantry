import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { PageHeader } from "@/components/editorial/PageHeader"
import { IngredientEditor } from "@/components/ingredients/IngredientEditor"
import { Skeleton } from "@/components/ui/skeleton"
import { useIngredient } from "@/lib/queries/ingredients"

export const Route = createFileRoute("/ingredients/$id/edit")({
  component: EditIngredientPage,
})

function EditIngredientPage() {
  const { t } = useTranslation()
  const { id } = Route.useParams()
  const navigate = useNavigate()
  const numericId = Number(id)

  const { data: ingredient, isLoading } = useIngredient(numericId)

  if (Number.isNaN(numericId)) {
    return (
      <p className="py-12 text-center text-on-surface-variant">
        {t("error.invalid_id")}
      </p>
    )
  }

  if (isLoading) {
    return (
      <div className="mx-auto max-w-6xl space-y-6 px-4 py-8 md:px-8 md:py-12">
        <Skeleton className="h-10 w-48" />
        <Skeleton className="h-10 w-full" />
        <div className="grid grid-cols-2 gap-4 md:grid-cols-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      </div>
    )
  }

  if (!ingredient) {
    return (
      <p className="py-12 text-center text-on-surface-variant">
        {t("error.not_found")}
      </p>
    )
  }

  return (
    <div className="mx-auto max-w-6xl space-y-8 px-4 py-8 md:px-8 md:py-12">
      <PageHeader title={t("ingredient.edit")} />
      <IngredientEditor
        ingredient={ingredient}
        onSuccess={() => navigate({ to: "/ingredients" })}
      />
    </div>
  )
}
