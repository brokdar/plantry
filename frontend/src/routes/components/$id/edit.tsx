import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { Skeleton } from "@/components/ui/skeleton"
import { ComponentEditor } from "@/components/components/ComponentEditor"
import { useComponent } from "@/lib/queries/components"

export const Route = createFileRoute("/components/$id/edit")({
  component: EditComponentPage,
})

function EditComponentPage() {
  const { t } = useTranslation()
  const { id } = Route.useParams()
  const navigate = useNavigate()
  const numericId = Number(id)

  const { data: component, isLoading } = useComponent(numericId)

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
        <Skeleton className="h-10 w-full" />
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

  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-semibold tracking-tight">
        {t("component.edit")}
      </h1>
      <ComponentEditor
        component={component}
        onSuccess={() =>
          navigate({
            to: "/components/$id",
            params: { id: String(component.id) },
          })
        }
      />
    </section>
  )
}
