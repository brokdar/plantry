import { createFileRoute, redirect } from "@tanstack/react-router"
import { z } from "zod"

import { SettingsShell } from "@/components/settings/SettingsShell"
import {
  DEFAULT_SETTINGS_SECTION,
  LEGACY_SETTINGS_TABS,
} from "@/components/settings/settings-sections"

const searchSchema = z.object({
  tab: z.string().optional(),
})

/** Phones (below 768 px) get the section list here; wider screens open a
 *  section next to it, so they go straight to one. */
function isPhone(): boolean {
  return (
    typeof window.matchMedia === "function" &&
    !window.matchMedia("(min-width: 768px)").matches
  )
}

export const Route = createFileRoute("/settings/")({
  validateSearch: (search) => searchSchema.parse(search),
  // Plan D20: legacy `?tab=` links map onto sections; `/settings` opens
  // You & household.
  beforeLoad: ({ search }) => {
    const legacy = search.tab ? LEGACY_SETTINGS_TABS[search.tab] : undefined
    if (legacy || !isPhone()) {
      throw redirect({
        to: "/settings/$section",
        params: { section: legacy ?? DEFAULT_SETTINGS_SECTION },
        replace: true,
      })
    }
  },
  component: () => <SettingsShell />,
})
