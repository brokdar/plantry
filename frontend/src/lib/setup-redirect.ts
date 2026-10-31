import { redirect } from "@tanstack/react-router"

import { getSetupStatus } from "@/lib/api/setup"
import { setupKeys } from "@/lib/queries/keys"
import { queryClient } from "@/lib/query-client"

/** `beforeLoad` guard for `/` and `/today`: a new install goes to the setup
 *  wizard. When the status fails to load, the page opens anyway. */
export async function redirectIfSetupIncomplete(): Promise<void> {
  const status = await queryClient
    .ensureQueryData({
      queryKey: setupKeys.status(),
      queryFn: getSetupStatus,
    })
    .catch(() => null)
  if (status?.completed === false) throw redirect({ to: "/setup" })
}
