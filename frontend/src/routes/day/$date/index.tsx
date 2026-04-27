import { addDays, subDays } from "date-fns"
import { ArrowLeft, ChevronLeft, ChevronRight } from "lucide-react"
import { useTranslation } from "react-i18next"
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"

import { Button } from "@/components/ui/button"
import { ReadOnlyPlannerGrid } from "@/components/planner/ReadOnlyPlannerGrid"
import { usePlatesByDate } from "@/lib/queries/plates"
import { useTimeSlots } from "@/lib/queries/slots"
import type { PlannerDay } from "@/components/planner/PlannerGrid"

export const Route = createFileRoute("/day/$date/")({
  component: DayPage,
})

function toYMD(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

function DayPage() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const { date } = Route.useParams()

  const platesQuery = usePlatesByDate(date)
  const slotsQuery = useTimeSlots(true)

  const plates = platesQuery.data?.plates ?? []
  const slots = slotsQuery.data?.items ?? []

  const dateObj = new Date(date + "T00:00:00")
  const weekday = new Intl.DateTimeFormat(i18n.language, {
    weekday: "long",
  }).format(dateObj)
  const dateFormatted = new Intl.DateTimeFormat(i18n.language, {
    month: "long",
    day: "numeric",
    year: "numeric",
  }).format(dateObj)

  const prevDate = toYMD(subDays(dateObj, 1))
  const nextDate = toYMD(addDays(dateObj, 1))

  // Build a single-day PlannerDay array for ReadOnlyPlannerGrid
  const weekday0Mon = (dateObj.getDay() + 6) % 7 // 0=Mon…6=Sun
  const days: PlannerDay[] = [
    {
      date,
      weekday: weekday0Mon,
      plates,
    },
  ]

  return (
    <div
      className="mx-auto max-w-5xl space-y-6 px-4 py-6 md:px-8 md:py-8"
      data-testid="day-page"
    >
      {/* Back link */}
      <Button asChild variant="ghost" size="sm" className="-ml-2">
        <Link
          to="/calendar"
          search={{ mode: "month", edit: false, search: "" }}
        >
          <ArrowLeft className="mr-1.5 size-4" />
          {t("day.back_to_calendar")}
        </Link>
      </Button>

      {/* Header row */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="font-heading text-2xl font-bold tracking-tight text-on-surface md:text-3xl">
          {t("day.title", { weekday, date: dateFormatted })}
        </h1>

        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            aria-label={t("day.prev_day")}
            onClick={() =>
              void navigate({ to: "/day/$date", params: { date: prevDate } })
            }
            className="h-8 w-8 text-on-surface-variant hover:text-on-surface"
          >
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            aria-label={t("day.next_day")}
            onClick={() =>
              void navigate({ to: "/day/$date", params: { date: nextDate } })
            }
            className="h-8 w-8 text-on-surface-variant hover:text-on-surface"
          >
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* Slots grid */}
      {platesQuery.isLoading || slotsQuery.isLoading ? (
        <p className="text-sm text-on-surface-variant">{t("common.loading")}</p>
      ) : plates.every((p) => p.components.length === 0) ? (
        <p className="py-12 text-center text-sm text-on-surface-variant">
          {t("day.empty")}
        </p>
      ) : (
        <ReadOnlyPlannerGrid days={days} slots={slots} />
      )}
    </div>
  )
}
