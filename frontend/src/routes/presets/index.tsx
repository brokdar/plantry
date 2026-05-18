import { createFileRoute } from "@tanstack/react-router"

import { PresetsPage } from "@/components/presets/PresetsPage"

export const Route = createFileRoute("/presets/")({
  component: PresetsRoute,
})

function PresetsRoute() {
  return <PresetsPage />
}
