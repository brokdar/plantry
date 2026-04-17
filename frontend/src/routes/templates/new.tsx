import { createFileRoute } from "@tanstack/react-router"

import { TemplateForm } from "@/components/templates/TemplateForm"

export const Route = createFileRoute("/templates/new")({
  component: NewTemplatePage,
})

function NewTemplatePage() {
  return <TemplateForm />
}
