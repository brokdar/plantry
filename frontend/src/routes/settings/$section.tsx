import { createFileRoute, redirect } from "@tanstack/react-router"

import { SettingsShell } from "@/components/settings/SettingsShell"
import {
  DEFAULT_SETTINGS_SECTION,
  isSettingsSection,
} from "@/components/settings/settings-sections"

export const Route = createFileRoute("/settings/$section")({
  beforeLoad: ({ params }) => {
    if (!isSettingsSection(params.section)) {
      throw redirect({
        to: "/settings/$section",
        params: { section: DEFAULT_SETTINGS_SECTION },
        replace: true,
      })
    }
  },
  component: SettingsSectionPage,
})

function SettingsSectionPage() {
  const { section } = Route.useParams()
  return (
    <SettingsShell
      section={isSettingsSection(section) ? section : DEFAULT_SETTINGS_SECTION}
    />
  )
}
