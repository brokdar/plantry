import { createFileRoute } from "@tanstack/react-router"
import { TodayPage } from "@/components/today/TodayPage"
import { useTodayISO } from "@/lib/hooks/useTodayISO"
import { redirectIfSetupIncomplete } from "@/lib/setup-redirect"

const DATE = /^\d{4}-\d{2}-\d{2}$/

export const Route = createFileRoute("/today/")({
  // `date` shows another day; without it, today.
  validateSearch: (search): { date?: string } => ({
    date:
      typeof search.date === "string" && DATE.test(search.date)
        ? search.date
        : undefined,
  }),
  beforeLoad: redirectIfSetupIncomplete,
  component: TodayRoute,
})

function TodayRoute() {
  const { date } = Route.useSearch()
  const today = useTodayISO()
  return <TodayPage date={date ?? today} today={today} />
}
