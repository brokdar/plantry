import { createFileRoute, Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { Skeleton } from "@/components/ui/skeleton"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Separator } from "@/components/ui/separator"
import { useComponent, useComponentNutrition } from "@/lib/queries/components"

export const Route = createFileRoute("/components/$id/")({
  component: ComponentDetailPage,
})

function ComponentDetailPage() {
  const { t } = useTranslation()
  const { id } = Route.useParams()
  const numericId = Number(id)

  const { data: component, isLoading } = useComponent(numericId)
  const { data: nutrition } = useComponentNutrition(numericId)

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
        <Skeleton className="h-24 w-full" />
      </section>
    )
  }

  if (!component) {
    return (
      <p className="py-12 text-center text-muted-foreground">
        {t("error.not_found")}
      </p>
    )
  }

  const nutritionFields = nutrition
    ? [
        { label: t("ingredient.kcal"), value: nutrition.kcal },
        { label: t("ingredient.protein"), value: nutrition.protein },
        { label: t("ingredient.fat"), value: nutrition.fat },
        { label: t("ingredient.carbs"), value: nutrition.carbs },
        { label: t("ingredient.fiber"), value: nutrition.fiber },
        { label: t("ingredient.sodium"), value: nutrition.sodium },
      ]
    : []

  return (
    <section className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            {component.name}
          </h1>
          <Badge variant="secondary" className="mt-1">
            {t(`component.role_${component.role}`)}
          </Badge>
        </div>
        <Button asChild>
          <Link to="/components/$id/edit" params={{ id: String(component.id) }}>
            {t("common.edit")}
          </Link>
        </Button>
      </div>

      <div className="flex gap-4 text-sm text-muted-foreground">
        <span>
          {t("component.reference_portions")}: {component.reference_portions}
        </span>
        {(component.prep_minutes > 0 || component.cook_minutes > 0) && (
          <span>
            {t("component.time", {
              prep: component.prep_minutes,
              cook: component.cook_minutes,
            })}
          </span>
        )}
      </div>

      {component.tags.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {component.tags.map((tag) => (
            <Badge key={tag} variant="outline">
              {tag}
            </Badge>
          ))}
        </div>
      )}

      {component.notes && (
        <p className="text-sm text-muted-foreground">{component.notes}</p>
      )}

      <Separator />

      {/* Ingredients */}
      {component.ingredients.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-sm font-medium">{t("component.ingredients")}</h3>
          <ul className="space-y-1 text-sm">
            {component.ingredients.map((ci) => (
              <li key={ci.id}>
                {ci.amount} {ci.unit} ({ci.grams}g)
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* Instructions */}
      {component.instructions.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-sm font-medium">{t("component.instructions")}</h3>
          <ol className="list-decimal space-y-1 pl-5 text-sm">
            {component.instructions.map((inst) => (
              <li key={inst.id}>{inst.text}</li>
            ))}
          </ol>
        </div>
      )}

      {/* Nutrition */}
      {nutritionFields.length > 0 && (
        <>
          <Separator />
          <div className="space-y-2">
            <h3 className="text-sm font-medium">{t("component.nutrition")}</h3>
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
              {nutritionFields.map((f) => (
                <div
                  key={f.label}
                  className="rounded-md border border-border bg-muted/50 px-3 py-2"
                >
                  <p className="text-xs text-muted-foreground">{f.label}</p>
                  <p className="text-sm font-medium">{f.value.toFixed(1)}</p>
                </div>
              ))}
            </div>
          </div>
        </>
      )}
    </section>
  )
}
