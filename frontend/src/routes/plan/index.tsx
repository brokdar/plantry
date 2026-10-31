import { createFileRoute } from "@tanstack/react-router"

import { PlanPage } from "@/components/plan/PlanPage"

const DATE = /^\d{4}-\d{2}-\d{2}$/

export const Route = createFileRoute("/plan/")({
  // `start` must be a window start (a shopping day); the server says so
  // with a 400 if it is not. Without it the anchored window opens.
  validateSearch: (search): { start?: string } => ({
    start:
      typeof search.start === "string" && DATE.test(search.start)
        ? search.start
        : undefined,
  }),
  component: PlanRoute,
})

function PlanRoute() {
  const { start } = Route.useSearch()
  return <PlanPage key={start ?? ""} start={start} />
}
