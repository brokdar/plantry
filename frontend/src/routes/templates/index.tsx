import { createFileRoute } from "@tanstack/react-router"

import { TemplateList } from "@/components/templates/TemplateList"

export const Route = createFileRoute("/templates/")({
  component: TemplatesPage,
})

function TemplatesPage() {
  return <TemplateList />
}
