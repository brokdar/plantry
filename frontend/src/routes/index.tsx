import {
  addWeeks,
  getISOWeek,
  getISOWeekYear,
  setISOWeek,
  setISOWeekYear,
} from "date-fns"
import { BarChart2, Download, Settings, Sparkles } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { createFileRoute, Link } from "@tanstack/react-router"

import { ChatPanel } from "@/components/chat/ChatPanel"
import { NutritionWeekSummary } from "@/components/planner/NutritionWeekSummary"
import { PlannerGrid } from "@/components/planner/PlannerGrid"
import { ShoppingPanel } from "@/components/planner/ShoppingPanel"
import { WeekNavigator } from "@/components/planner/WeekNavigator"
import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { useAISettings } from "@/lib/queries/ai"
import { useTimeSlots } from "@/lib/queries/slots"
import {
  useCopyWeek,
  useWeekByDate,
  useWeekNutrition,
} from "@/lib/queries/weeks"
import { useChatUI } from "@/lib/stores/chat-ui"
import { toastError } from "@/lib/toast"

export const Route = createFileRoute("/")({
  component: PlannerPage,
  staticData: { shellVariant: "rail" as const },
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
  const openChat = useChatUI((s) => s.setOpen)

  const slotsQuery = useTimeSlots(true)
  const weekQuery = useWeekByDate(year, week)
  const copyMut = useCopyWeek()
  const { data: aiSettings } = useAISettings()
  const nutritionQuery = useWeekNutrition(weekQuery.data?.id ?? 0)

  const slots = slotsQuery.data?.items ?? []

  if (slotsQuery.isLoading || weekQuery.isLoading) {
    return (
      <div className="px-6 py-12 md:px-12">
        <p className="text-sm text-on-surface-variant">{t("common.loading")}</p>
      </div>
    )
  }

  if (slots.length === 0) {
    return (
      <div className="px-6 py-12 md:px-12">
        <section className="editorial-shadow mx-auto flex max-w-2xl flex-col items-center gap-4 rounded-2xl bg-surface-container-lowest py-16 text-center">
          <Settings className="size-10 text-on-surface-variant" aria-hidden />
          <h2 className="font-heading text-2xl font-bold text-on-surface">
            {t("planner.empty_state_no_slots_title")}
          </h2>
          <p className="max-w-md text-sm text-on-surface-variant">
            {t("planner.empty_state_no_slots_body")}
          </p>
          <Button asChild>
            <Link to="/settings">{t("planner.empty_state_no_slots_cta")}</Link>
          </Button>
        </section>
      </div>
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
      toastError(err, t)
    }
  }

  const dailyAvgKcal = (() => {
    const days = nutritionQuery.data?.days
    if (!days?.length) return null
    const plannedDays = days.filter((d) => d.macros.kcal > 0)
    if (!plannedDays.length) return null
    const avg =
      plannedDays.reduce((acc, d) => acc + d.macros.kcal, 0) /
      plannedDays.length
    return Math.round(avg)
  })()

  return (
    <div className="px-4 py-6 md:px-8 md:py-8">
      <header
        className="mb-10 flex flex-col justify-between gap-8 md:flex-row md:items-end"
        data-testid="page-header"
      >
        <div className="max-w-2xl">
          <h1 className="font-heading text-4xl leading-tight font-extrabold tracking-tight text-on-surface md:text-5xl">
            {t("planner.title")}
          </h1>
          <p className="mt-2 text-sm font-medium text-on-surface-variant">
            {t("planner.week_label", {
              week: week_.week_number,
              year: week_.year,
            })}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          {dailyAvgKcal !== null && (
            <div className="flex flex-col justify-center rounded-xl bg-surface-container-low px-5 py-3">
              <span className="text-[9px] font-bold tracking-widest text-on-surface-variant uppercase">
                {t("planner.daily_avg")}
              </span>
              <span className="font-heading text-xl font-bold text-primary">
                {dailyAvgKcal.toLocaleString()} kcal
              </span>
            </div>
          )}
          <Button
            variant="ghost"
            size="icon"
            onClick={() => setNutritionOpen(true)}
            aria-label={t("nutrition.button")}
          >
            <BarChart2 className="size-4" />
          </Button>
          {aiSettings?.enabled && (
            <Button
              variant="ghost"
              size="icon"
              onClick={() => openChat(true)}
              aria-label={t("chat.button")}
              data-testid="chat-open-button"
            >
              <Sparkles className="size-4" />
            </Button>
          )}
          <WeekNavigator
            year={week_.year}
            weekNumber={week_.week_number}
            onPrev={() => setYearWeek(shiftWeek(year, week, -1))}
            onNext={() => setYearWeek(shiftWeek(year, week, 1))}
            onCopy={handleCopy}
          />
          <Button
            onClick={() => setShoppingOpen(true)}
            className="gradient-primary editorial-shadow border-0 text-on-primary hover:opacity-90"
          >
            <Download className="mr-1.5 size-4" />
            {t("shopping.button")}
          </Button>
        </div>
      </header>

      <div className="-mx-2 md:-mx-4">
        <PlannerGrid week={week_} slots={slots} />
      </div>

      <ShoppingPanel
        weekId={week_.id}
        open={shoppingOpen}
        onOpenChange={setShoppingOpen}
      />

      <Sheet open={nutritionOpen} onOpenChange={setNutritionOpen}>
        <SheetContent side="right" className="flex w-full flex-col sm:max-w-sm">
          <SheetHeader>
            <SheetTitle className="flex items-center gap-2">
              <BarChart2 className="size-4" />
              {t("nutrition.title")}
            </SheetTitle>
          </SheetHeader>
          <NutritionWeekSummary weekId={week_.id} />
        </SheetContent>
      </Sheet>

      <ChatPanel weekId={week_.id} />
    </div>
  )
}
