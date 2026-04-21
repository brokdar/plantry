import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { ChevronLeft } from "lucide-react"

import { ComponentEditor } from "@/components/components/ComponentEditor"
import { NewComponentPathChooser } from "@/components/components/NewComponentPathChooser"
import { PageHeader } from "@/components/editorial/PageHeader"

export const Route = createFileRoute("/components/new")({
  component: NewComponentPage,
})

function NewComponentPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()

  return (
    <div className="mx-auto max-w-6xl space-y-8 px-4 py-8 md:px-8 md:py-12">
      <PageHeader
        eyebrow={t("component.edit_eyebrow")}
        title={t("component.new_title")}
        description={t("component.new_subtitle")}
        breadcrumb={
          <Link
            to="/components"
            className="inline-flex items-center gap-1 hover:text-on-surface"
          >
            <ChevronLeft className="size-3" aria-hidden />
            {t("component.breadcrumb_back")}
          </Link>
        }
      />
      <NewComponentPathChooser />
      <ComponentEditor onSuccess={() => navigate({ to: "/components" })} />
    </div>
  )
}
