import { createFileRoute } from "@tanstack/react-router"

import { InsightsPage } from "@/components/settings/insights/InsightsPage"

// A static child of /settings, so it wins over `/settings/$section` (D37).
// `run` opens that run's detail sheet.
export const Route = createFileRoute("/settings/assistant/insights")({
  validateSearch: (search): { run?: number } => ({
    run:
      typeof search.run === "number" &&
      Number.isInteger(search.run) &&
      search.run > 0
        ? search.run
        : undefined,
  }),
  component: InsightsRoute,
})

function InsightsRoute() {
  const { run } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <InsightsPage
      runId={run}
      onRunChange={(id) => navigate({ search: { run: id } })}
    />
  )
}
