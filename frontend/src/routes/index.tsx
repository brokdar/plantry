import {
  addWeeks,
  getISOWeek,
  getISOWeekYear,
  setISOWeek,
  setISOWeekYear,
} from "date-fns"
import { Settings } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { createFileRoute, Link } from "@tanstack/react-router"

import { PlannerGrid } from "@/components/planner/PlannerGrid"
import { WeekNavigator } from "@/components/planner/WeekNavigator"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { ApiError } from "@/lib/api/client"
import { useTimeSlots } from "@/lib/queries/slots"
import { useCopyWeek, useWeekByDate } from "@/lib/queries/weeks"

export const Route = createFileRoute("/")({
  component: PlannerPage,
})

function nowYearWeek() {
  const now = new Date()
  return { year: getISOWeekYear(now), week: getISOWeek(now) }
}

function shiftWeek(year: number, week: number, delta: number) {
  let d = setISOWeekYear(new Date(), year)
  d = setISOWeek(d, week)
  d = addWeeks(d, delta)
  return { year: getISOWeekYear(d), week: getISOWeek(d) }
}

function PlannerPage() {
  const { t } = useTranslation()
  const [{ year, week }, setYearWeek] = useState(nowYearWeek)

  const slotsQuery = useTimeSlots(true)
  const weekQuery = useWeekByDate(year, week)
  const copyMut = useCopyWeek()

  const slots = slotsQuery.data?.items ?? []

  if (slotsQuery.isLoading || weekQuery.isLoading) {
    return (
      <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
    )
  }

  if (slots.length === 0) {
    return (
      <Card>
        <CardContent className="flex flex-col items-center gap-4 py-12 text-center">
          <Settings className="h-10 w-10 text-muted-foreground" aria-hidden />
          <h2 className="text-xl font-semibold">
            {t("planner.empty_state_no_slots_title")}
          </h2>
          <p className="max-w-md text-sm text-muted-foreground">
            {t("planner.empty_state_no_slots_body")}
          </p>
          <Button asChild>
            <Link to="/settings">{t("planner.empty_state_no_slots_cta")}</Link>
          </Button>
        </CardContent>
      </Card>
    )
  }

  const week_ = weekQuery.data
  if (!week_) return null

  async function handleCopy() {
    if (!week_) return
    const next = shiftWeek(year, week, 1)
    try {
      await copyMut.mutateAsync({
        id: week_.id,
        input: { target_year: next.year, target_week: next.week },
      })
      setYearWeek(next)
    } catch (err) {
      window.alert(
        err instanceof ApiError ? t(err.messageKey) : t("error.server")
      )
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">
          {t("planner.title")}
        </h1>
        <WeekNavigator
          year={week_.year}
          weekNumber={week_.week_number}
          onPrev={() => setYearWeek(shiftWeek(year, week, -1))}
          onNext={() => setYearWeek(shiftWeek(year, week, 1))}
          onCopy={handleCopy}
        />
      </div>
      <PlannerGrid week={week_} slots={slots} />
    </div>
  )
}
