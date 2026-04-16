import { createFileRoute } from "@tanstack/react-router"

import { TimeSlotsEditor } from "@/components/settings/TimeSlotsEditor"

export const Route = createFileRoute("/settings/")({
  component: SettingsPage,
})

function SettingsPage() {
  return <TimeSlotsEditor />
}
