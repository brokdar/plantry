import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { ApiError } from "@/lib/api/client"
import { useFood } from "@/lib/queries/foods"

import { RecipeDetail } from "./RecipeDetail"

/** Loads a recipe by its route id and renders the detail, or a not-found /
 *  load-error state with a way back to Recipes. */
export function RecipeDetailPage({
  id,
  portions,
}: {
  id: string
  portions?: number
}) {
  const { t } = useTranslation()
  const numericId = Number(id)

  const { data: food, isLoading, isError, error, refetch } = useFood(numericId)

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

  if (isError) {
    const missing = error instanceof ApiError && error.status === 404
    return missing ? (
      <RecipeUnavailable reason="not_found" />
    ) : (
      <RecipeUnavailable reason="load_error" onRetry={() => void refetch()} />
    )
  }

  if (!food || food.kind !== "composed") {
    return <RecipeUnavailable reason="not_found" />
  }

  return <RecipeDetail key={food.id} recipe={food} portions={portions} />
}

function RecipeUnavailable({
  reason,
  onRetry,
}: {
  reason: "not_found" | "load_error"
  onRetry?: () => void
}) {
  const { t } = useTranslation()
  return (
    <div
      role={reason === "load_error" ? "alert" : undefined}
      className="mx-auto flex max-w-xl flex-col items-start gap-3 px-4 py-12 text-ink md:px-8"
      data-testid={`recipe-${reason.replace("_", "-")}`}
    >
      <h1 className="font-heading text-h1 font-semibold">
        {t(`recipe_detail.${reason}_title`)}
      </h1>
      <p className="text-ink-muted">{t(`recipe_detail.${reason}_body`)}</p>
      <div className="mt-2 flex flex-wrap gap-3">
        {onRetry && (
          <Button type="button" onClick={onRetry}>
            {t("common.retry")}
          </Button>
        )}
        <Button asChild variant={onRetry ? "outline" : "default"}>
          <Link to="/recipes">{t("recipe_detail.back")}</Link>
        </Button>
      </div>
    </div>
  )
}
