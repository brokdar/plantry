import { createFileRoute, redirect } from "@tanstack/react-router"

import { getSetupStatus } from "@/lib/api/setup"
import { setupKeys } from "@/lib/queries/keys"
import { queryClient } from "@/lib/query-client"

export const Route = createFileRoute("/")({
  // Plan D2: only `/` sends a new install to the setup wizard; deep links
  // keep working. Otherwise, and when the status fails to load, `/` opens
  // the plan board.
  beforeLoad: async () => {
    const status = await queryClient
      .ensureQueryData({
        queryKey: setupKeys.status(),
        queryFn: getSetupStatus,
      })
      .catch(() => null)
    if (status?.completed === false) throw redirect({ to: "/setup" })
    throw redirect({ to: "/plan" })
  },
})
