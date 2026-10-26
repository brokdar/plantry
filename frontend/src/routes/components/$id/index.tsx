import { createFileRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { RecipeDetail } from "@/components/components/RecipeDetail"
import { Skeleton } from "@/components/ui/skeleton"
import { useFood } from "@/lib/queries/foods"

export const Route = createFileRoute("/components/$id/")({
  component: RecipeDetailPage,
})

function RecipeDetailPage() {
  const { t } = useTranslation()
  const { id } = Route.useParams()
  const numericId = Number(id)

  const { data: food, isLoading } = useFood(numericId)

  if (Number.isNaN(numericId)) {
    return (
      <p className="py-12 text-center text-ink-muted">
        {t("error.invalid_id")}
      </p>
    )
  }

  if (isLoading) {
    return (
      <div className="mx-auto flex max-w-5xl flex-col gap-6 px-4 py-8 md:px-8">
        <Skeleton className="h-8 w-40" />
        <Skeleton className="aspect-[4/3] w-full md:w-5/12" />
        <Skeleton className="h-10 w-2/3" />
      </div>
    )
  }

  if (!food || food.kind !== "composed") {
    return (
      <p className="py-12 text-center text-ink-muted">{t("error.not_found")}</p>
    )
  }

  return <RecipeDetail recipe={food} />
}
