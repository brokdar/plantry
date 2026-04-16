import {
  addWeeks,
  getISOWeek,
  getISOWeekYear,
  setISOWeek,
  setISOWeekYear,
} from "date-fns"
import { BarChart2, Settings, ShoppingCart } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { createFileRoute, Link } from "@tanstack/react-router"

import { NutritionWeekSummary } from "@/components/planner/NutritionWeekSummary"
import { PlannerGrid } from "@/components/planner/PlannerGrid"
import { ShoppingPanel } from "@/components/planner/ShoppingPanel"
import { WeekNavigator } from "@/components/planner/WeekNavigator"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
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
  const [shoppingOpen, setShoppingOpen] = useState(false)
  const [nutritionOpen, setNutritionOpen] = useState(false)

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
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-2xl font-semibold tracking-tight">
          {t("planner.title")}
        </h1>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setShoppingOpen(true)}
          >
            <ShoppingCart className="mr-1.5 h-4 w-4" />
            {t("shopping.button")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setNutritionOpen(true)}
          >
            <BarChart2 className="mr-1.5 h-4 w-4" />
            {t("nutrition.button")}
          </Button>
          <WeekNavigator
            year={week_.year}
            weekNumber={week_.week_number}
            onPrev={() => setYearWeek(shiftWeek(year, week, -1))}
            onNext={() => setYearWeek(shiftWeek(year, week, 1))}
            onCopy={handleCopy}
          />
        </div>
      </div>
      <PlannerGrid week={week_} slots={slots} />

      <ShoppingPanel
        weekId={week_.id}
        open={shoppingOpen}
        onOpenChange={setShoppingOpen}
      />

      <Sheet open={nutritionOpen} onOpenChange={setNutritionOpen}>
        <SheetContent side="right" className="flex w-full flex-col sm:max-w-sm">
          <SheetHeader>
            <SheetTitle className="flex items-center gap-2">
              <BarChart2 className="h-4 w-4" />
              {t("nutrition.title")}
            </SheetTitle>
          </SheetHeader>
          <NutritionWeekSummary weekId={week_.id} />
        </SheetContent>
      </Sheet>
    </div>
  )
}
