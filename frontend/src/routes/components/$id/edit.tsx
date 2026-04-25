import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { ComponentEditor } from "@/components/components/ComponentEditor"
import { PageHeader } from "@/components/editorial/PageHeader"
import { Skeleton } from "@/components/ui/skeleton"
import { useFood } from "@/lib/queries/foods"

export const Route = createFileRoute("/components/$id/edit")({
  component: EditComponentPage,
})

function EditComponentPage() {
  const { t } = useTranslation()
  const { id } = Route.useParams()
  const navigate = useNavigate()
  const numericId = Number(id)

  const { data: component, isLoading } = useFood(numericId)

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

  if (!component) {
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
      <ComponentEditor
        component={component}
        onSuccess={() => navigate({ to: "/components" })}
        onDeleted={() => navigate({ to: "/components" })}
      />
    </div>
  )
}
