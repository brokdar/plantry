import { ArrowLeft } from "lucide-react"
import { useTranslation } from "react-i18next"
import { createFileRoute, Link } from "@tanstack/react-router"

import { ReadOnlyPlannerGrid } from "@/components/planner/ReadOnlyPlannerGrid"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useTimeSlots } from "@/lib/queries/slots"
import { useWeek } from "@/lib/queries/weeks"

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
    <section className="flex flex-col gap-6" data-testid="archive-detail">
      <div className="flex items-center gap-3">
        <Button asChild variant="ghost" size="sm">
          <Link to="/archive">
            <ArrowLeft className="mr-1.5 h-4 w-4" />
            {t("archive.back")}
          </Link>
        </Button>
      </div>

      {(weekQuery.isLoading || slotsQuery.isLoading) && (
        <Skeleton className="h-64 w-full" />
      )}

      {week && (
        <div className="flex flex-col gap-4">
          <h1 className="text-2xl font-semibold tracking-tight">
            {t("archive.week_label", {
              week: week.week_number,
              year: week.year,
            })}
          </h1>
          <ReadOnlyPlannerGrid week={week} slots={slots} />
        </div>
      )}
    </section>
  )
}
