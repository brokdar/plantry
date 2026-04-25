import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { ChevronLeft } from "lucide-react"

import { PageHeader } from "@/components/editorial/PageHeader"
import { IngredientEditor } from "@/components/ingredients/IngredientEditor"
import { Skeleton } from "@/components/ui/skeleton"
import type { LeafFood } from "@/lib/api/foods"
import { useFood } from "@/lib/queries/foods"

export const Route = createFileRoute("/ingredients/$id/edit")({
  component: EditIngredientPage,
})

function EditIngredientPage() {
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
        <div className="grid grid-cols-2 gap-4 md:grid-cols-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
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
    <div className="mx-auto max-w-6xl space-y-8 px-4 pt-8 pb-32 md:px-8 md:pt-12 md:pb-16">
      <PageHeader
        eyebrow={t("ingredient.edit_eyebrow")}
        title={t("ingredient.edit_title")}
        description={t("ingredient.edit_subtitle")}
        breadcrumb={
          <Link
            to="/ingredients"
            className="inline-flex items-center gap-1 hover:text-on-surface"
          >
            <ChevronLeft className="size-3" aria-hidden />
            {t("ingredient.breadcrumb_back")}
          </Link>
        }
      />
      <IngredientEditor
        ingredient={food as LeafFood}
        onSuccess={() => navigate({ to: "/ingredients" })}
        onDeleted={() => navigate({ to: "/ingredients" })}
      />
    </div>
  )
}
