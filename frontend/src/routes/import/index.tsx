import { createFileRoute } from "@tanstack/react-router"

import { ImportWizard } from "@/components/import/ImportWizard"

export const Route = createFileRoute("/import/")({
  component: ImportWizard,
})
