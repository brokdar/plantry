import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { ComponentEditor } from "@/components/components/ComponentEditor"

export const Route = createFileRoute("/components/new")({
  component: NewComponentPage,
})

function NewComponentPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()

  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-semibold tracking-tight">
        {t("component.create")}
      </h1>
      <ComponentEditor onSuccess={() => navigate({ to: "/components" })} />
    </section>
  )
}
