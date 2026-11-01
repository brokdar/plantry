import { createFileRoute, redirect } from "@tanstack/react-router"

import { PlanPage } from "@/components/plan/PlanPage"
import { useIsPhone } from "@/components/plan/use-is-phone"
import { PlanWizard } from "@/components/plan-wizard/PlanWizard"
import { getSetupStatus } from "@/lib/api/setup"
import i18n from "@/lib/i18n"
import { setupKeys } from "@/lib/queries/keys"
import { queryClient } from "@/lib/query-client"
import { toast } from "@/lib/toast"

const DATE = /^\d{4}-\d{2}-\d{2}$/

export const Route = createFileRoute("/plan/new")({
  // `start` must be a window start; without it the anchored window.
  validateSearch: (search): { start?: string } => ({
    start:
      typeof search.start === "string" && DATE.test(search.start)
        ? search.start
        : undefined,
  }),
  // Locked below the recipe target (D32): the library is where to go.
  beforeLoad: async () => {
    const status = await queryClient
      .ensureQueryData({
        queryKey: setupKeys.status(),
        queryFn: getSetupStatus,
        retry: false,
      })
      .catch(() => null)
    if (status && status.recipe_count < status.recipe_target) {
      toast(
        i18n.t("nav.generate_locked", {
          count: status.recipe_target - status.recipe_count,
        })
      )
      throw redirect({ to: "/recipes" })
    }
  },
  component: PlanNewRoute,
})

function PlanNewRoute() {
  const { start } = Route.useSearch()
  const phone = useIsPhone()
  // From 768 px the wizard is a dialog over the board of its window (D31).
  return (
    <>
      {!phone && <PlanPage key={start ?? ""} start={start} />}
      <PlanWizard key={start ?? ""} start={start} />
    </>
  )
}
