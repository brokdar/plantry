import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { RecipeEditor } from "@/components/recipes/RecipeEditor"
import { PageHeader } from "@/components/editorial/PageHeader"
import { Skeleton } from "@/components/ui/skeleton"
import type { ComposedFood } from "@/lib/api/foods"
import { useFood } from "@/lib/queries/foods"

export const Route = createFileRoute("/recipes/$id/edit")({
  component: EditComponentPage,
})

function EditComponentPage() {
  const { t } = useTranslation()
  const { id } = Route.useParams()
  const navigate = useNavigate()
  const numericId = Number(id)

  const { data: food, isLoading } = useFood(numericId)

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
        <Skeleton className="h-10 w-full" />
      </div>
    )
  }

  if (!food) {
    return (
      <p className="py-12 text-center text-on-surface-variant">
        {t("error.not_found")}
      </p>
    )
  }

  return (
    <div className="mx-auto max-w-6xl space-y-8 px-4 py-8 md:px-8 md:py-12">
      <PageHeader
        eyebrow={t("component.edit_eyebrow")}
        title={t("component.edit_title")}
        description={t("component.edit_subtitle")}
      />
      <RecipeEditor
        component={food as ComposedFood}
        onSuccess={() =>
          navigate({ to: "/recipes/$id", params: { id: String(food.id) } })
        }
        onDeleted={() => navigate({ to: "/recipes" })}
      />
    </div>
  )
}
