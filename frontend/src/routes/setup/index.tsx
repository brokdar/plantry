import { createFileRoute } from "@tanstack/react-router"

import { SetupWizard } from "@/components/setup/SetupWizard"

export const Route = createFileRoute("/setup/")({
  component: SetupWizard,
})
