import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { Skeleton } from "@/components/ui/skeleton"
import { IngredientEditor } from "@/components/ingredients/IngredientEditor"
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
      <p className="py-12 text-center text-muted-foreground">
        {t("error.invalid_id")}
      </p>
    )
  }

  if (isLoading) {
    return (
      <section className="space-y-6">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-10 w-full" />
        <div className="grid grid-cols-2 gap-4 md:grid-cols-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      </section>
    )
  }

  if (!ingredient) {
    return (
      <p className="py-12 text-center text-muted-foreground">
        {t("error.not_found")}
      </p>
    )
  }

  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-semibold tracking-tight">
        {t("ingredient.edit")}
      </h1>
      <IngredientEditor
        ingredient={ingredient}
        onSuccess={() => navigate({ to: "/ingredients" })}
      />
    </section>
  )
}
