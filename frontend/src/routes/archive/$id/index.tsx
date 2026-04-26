import { ArrowLeft } from "lucide-react"
import { useTranslation } from "react-i18next"
import { createFileRoute, Link } from "@tanstack/react-router"

import { CopyToCurrentButton } from "@/components/archive/CopyToCurrentButton"
import type { PlannerDay } from "@/components/planner/PlannerGrid"
import { ReadOnlyPlannerGrid } from "@/components/planner/ReadOnlyPlannerGrid"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useTimeSlots } from "@/lib/queries/slots"
import { useWeek } from "@/lib/queries/weeks"
import type { Week } from "@/lib/api/weeks"

function isoWeekMonday(year: number, week: number): Date {
  const jan4 = new Date(Date.UTC(year, 0, 4))
  const dow = jan4.getUTCDay() || 7
  const monday = new Date(jan4)
  monday.setUTCDate(jan4.getUTCDate() - (dow - 1) + (week - 1) * 7)
  return monday
}

function weekToPlannerDays(week: Week): PlannerDay[] {
  const monday = isoWeekMonday(week.year, week.week_number)
  return Array.from({ length: 7 }, (_, i) => {
    const d = new Date(monday)
    d.setUTCDate(monday.getUTCDate() + i)
    const dateStr = `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}-${String(d.getUTCDate()).padStart(2, "0")}`
    return {
      date: dateStr,
      weekday: i,
      plates: week.plates.filter((p) => p.day === i),
    }
  })
}

export const Route = createFileRoute("/archive/$id/")({
  component: ArchiveDetailPage,
})

function ArchiveDetailPage() {
  const { t } = useTranslation()
  const { id } = Route.useParams()
  const numericId = Number(id)

  const slotsQuery = useTimeSlots(false)
  const weekQuery = useWeek(numericId)

  if (Number.isNaN(numericId)) {
    return (
      <p className="py-12 text-center text-muted-foreground">
        {t("error.invalid_id")}
      </p>
    )
  }

  const slots = slotsQuery.data?.items ?? []
  const week = weekQuery.data

  return (
    <div
      className="mx-auto max-w-6xl space-y-6 px-4 py-8 md:px-8 md:py-12"
      data-testid="archive-detail"
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Button asChild variant="ghost" size="sm">
          <Link to="/archive">
            <ArrowLeft className="mr-1.5 size-4" />
            {t("archive.back")}
          </Link>
        </Button>
        {week && (
          <CopyToCurrentButton
            weekId={week.id}
            testId={`copy-to-current-detail-${week.id}`}
          />
        )}
      </div>

      {(weekQuery.isLoading || slotsQuery.isLoading) && (
        <Skeleton className="h-64 w-full" />
      )}

      {week && (
        <div className="flex flex-col gap-4">
          <h1 className="font-heading text-3xl font-extrabold tracking-tight text-on-surface md:text-4xl">
            {t("archive.week_label", {
              week: week.week_number,
              year: week.year,
            })}
          </h1>
          <div className="editorial-shadow overflow-hidden rounded-2xl bg-surface-container-lowest p-4 md:p-6">
            <ReadOnlyPlannerGrid days={weekToPlannerDays(week)} slots={slots} />
          </div>
        </div>
      )}
    </div>
  )
}
