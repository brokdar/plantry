import { createFileRoute, redirect } from "@tanstack/react-router"

import { SettingsShell } from "@/components/settings/SettingsShell"
import { DEFAULT_SETTINGS_SECTION } from "@/components/settings/settings-sections"

/** Phones (below 768 px) get the section list here; wider screens open a
 *  section next to it, so they go straight to one. */
function isPhone(): boolean {
  return (
    typeof window.matchMedia === "function" &&
    !window.matchMedia("(min-width: 768px)").matches
  )
}

export const Route = createFileRoute("/settings/")({
  // Plan D20: `/settings` opens You & household.
  beforeLoad: () => {
    if (!isPhone()) {
      throw redirect({
        to: "/settings/$section",
        params: { section: DEFAULT_SETTINGS_SECTION },
        replace: true,
      })
    }
  },
  component: () => <SettingsShell />,
})
