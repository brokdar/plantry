import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { SettingsShell } from "@/components/settings/SettingsShell"

const searchSchema = z.object({
  tab: z
    .enum(["general", "plan", "ai", "nutrition", "meal_slots", "system"])
    .optional(),
})

export const Route = createFileRoute("/settings/")({
  component: SettingsShell,
  validateSearch: (search) => searchSchema.parse(search),
})
