import { createFileRoute } from "@tanstack/react-router"
import { TodayPage } from "@/components/today/TodayPage"
import { isValidYMD } from "@/lib/dates"
import { useTodayISO } from "@/lib/hooks/useTodayISO"
import { redirectIfSetupIncomplete } from "@/lib/setup-redirect"

export const Route = createFileRoute("/today/")({
  // `date` shows another day; without it, today.
  validateSearch: (search): { date?: string } => ({
    date:
      typeof search.date === "string" && isValidYMD(search.date)
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
