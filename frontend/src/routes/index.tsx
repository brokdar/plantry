import { createFileRoute, redirect } from "@tanstack/react-router"

import { redirectIfSetupIncomplete } from "@/lib/setup-redirect"

export const Route = createFileRoute("/")({
  // A new install goes to the setup wizard; otherwise, and when the status
  // fails to load, `/` opens Today (plan D1).
  beforeLoad: async () => {
    await redirectIfSetupIncomplete()
    throw redirect({ to: "/today" })
  },
})
