import { createFileRoute } from "@tanstack/react-router"

import { PlanPage } from "@/components/plan/PlanPage"

const DATE = /^\d{4}-\d{2}-\d{2}$/
const CELL = /^\d{4}-\d{2}-\d{2}\|\d+$/

export const Route = createFileRoute("/plan/")({
  // `start` must be a window start (a shopping day); the server says so
  // with a 400 if it is not. Without it the anchored window opens.
  // `cell` (`date|slotId`) opens that cell's sheet, e.g. a batch's cook
  // cell from a leftover in the next window. `day` focuses that day's
  // first cell (Today's week strip). `run` is the generation the board
  // follows (D35), set by useGenerate/useRegenerate.
  validateSearch: (
    search
  ): { start?: string; cell?: string; day?: string; run?: number } => ({
    start:
      typeof search.start === "string" && DATE.test(search.start)
        ? search.start
        : undefined,
    cell:
      typeof search.cell === "string" && CELL.test(search.cell)
        ? search.cell
        : undefined,
    day:
      typeof search.day === "string" && DATE.test(search.day)
        ? search.day
        : undefined,
    run:
      typeof search.run === "number" &&
      Number.isInteger(search.run) &&
      search.run > 0
        ? search.run
        : undefined,
  }),
  component: PlanRoute,
})

function PlanRoute() {
  const { start, cell, day } = Route.useSearch()
  return <PlanPage key={start ?? ""} start={start} cell={cell} day={day} />
}
