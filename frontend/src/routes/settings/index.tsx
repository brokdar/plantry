import { createFileRoute } from "@tanstack/react-router"

import { ProfileEditor } from "@/components/settings/ProfileEditor"
import { TimeSlotsEditor } from "@/components/settings/TimeSlotsEditor"
import { Separator } from "@/components/ui/separator"

export const Route = createFileRoute("/settings/")({
  component: SettingsPage,
})

function SettingsPage() {
  return (
    <div className="mx-auto max-w-2xl space-y-8 p-4">
      <ProfileEditor />
      <Separator />
      <TimeSlotsEditor />
    </div>
  )
}
