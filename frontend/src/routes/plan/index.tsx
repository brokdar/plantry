import { createFileRoute } from "@tanstack/react-router"

import { PlanPage } from "@/components/plan/PlanPage"

const DATE = /^\d{4}-\d{2}-\d{2}$/
const CELL = /^\d{4}-\d{2}-\d{2}\|\d+$/

export const Route = createFileRoute("/plan/")({
  // `start` must be a window start (a shopping day); the server says so
  // with a 400 if it is not. Without it the anchored window opens.
  // `cell` (`date|slotId`) opens that cell's sheet, e.g. a batch's cook
  // cell from a leftover in the next window.
  validateSearch: (search): { start?: string; cell?: string } => ({
    start:
      typeof search.start === "string" && DATE.test(search.start)
        ? search.start
        : undefined,
    cell:
      typeof search.cell === "string" && CELL.test(search.cell)
        ? search.cell
        : undefined,
  }),
  component: PlanRoute,
})

function PlanRoute() {
  const { start, cell } = Route.useSearch()
  return <PlanPage key={start ?? ""} start={start} cell={cell} />
}
