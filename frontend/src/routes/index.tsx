import {
  addWeeks,
  getISOWeek,
  getISOWeekYear,
  setISOWeek,
  setISOWeekYear,
} from "date-fns"
import { BarChart2, Download, Settings, Sparkles } from "lucide-react"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { createFileRoute, Link } from "@tanstack/react-router"

import { ChatPanel } from "@/components/chat/ChatPanel"
import { PageHeader } from "@/components/editorial/PageHeader"
import { FillEmptySlotsButton } from "@/components/planner/FillEmptySlotsButton"
import { MobilePlannerGrid } from "@/components/planner/MobilePlannerGrid"
import { NutritionWeekSummary } from "@/components/planner/NutritionWeekSummary"
import { PlannerGrid } from "@/components/planner/PlannerGrid"
import { RevertBanner } from "@/components/planner/RevertBanner"
import { ShoppingPanel } from "@/components/planner/ShoppingPanel"
import { WeekNavigator } from "@/components/planner/WeekNavigator"
import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { clearWeekPlates } from "@/lib/api/weeks"
import { useAISettings } from "@/lib/queries/ai"
import { queryClient } from "@/lib/query-client"
import { weekKeys } from "@/lib/queries/keys"
import { useTimeSlots } from "@/lib/queries/slots"
import {
  useCopyWeek,
  useWeekByDate,
  useWeekNutrition,
} from "@/lib/queries/weeks"
import { useChatUI } from "@/lib/stores/chat-ui"
import { usePlannerUI } from "@/lib/stores/planner-ui"
import { toastError } from "@/lib/toast"

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
  const openChat = useChatUI((s) => s.setOpen)

  const slotsQuery = useTimeSlots(true)
  const weekQuery = useWeekByDate(year, week)
  const copyMut = useCopyWeek()
  const { data: aiSettings } = useAISettings()
  const nutritionQuery = useWeekNutrition(weekQuery.data?.id ?? 0)

  const aiFill = usePlannerUI((s) => s.aiFill)
  const recordAiFilledPlate = usePlannerUI((s) => s.recordAiFilledPlate)
  const dismissAiFillBanner = usePlannerUI((s) => s.dismissAiFillBanner)
  const endAiFillSession = usePlannerUI((s) => s.endAiFillSession)

  // Watch plates created after the fill session started. Plates with
  // created_at newer than aiFill.startedAt are attributed to the kitchen agent
  // for the duration of this session. Refresh clears the session entirely.
  useEffect(() => {
    if (!aiFill || aiFill.weekId !== weekQuery.data?.id) return
    const plates = weekQuery.data?.plates ?? []
    for (const p of plates) {
      const created = Date.parse(p.created_at)
      if (!Number.isNaN(created) && created >= aiFill.startedAt) {
        recordAiFilledPlate(p.id)
      }
    }
  }, [aiFill, weekQuery.data, recordAiFilledPlate])

  async function handleRevert() {
    if (!weekQuery.data) return
    try {
      await clearWeekPlates(weekQuery.data.id)
      endAiFillSession()
      await queryClient.invalidateQueries({ queryKey: weekKeys.all })
    } catch (err) {
      toastError(err, t)
    }
  }

  const slots = slotsQuery.data?.items ?? []

  if (slotsQuery.isLoading || weekQuery.isLoading) {
    return (
      <div className="mx-auto max-w-7xl px-4 py-8 md:px-8 md:py-12">
        <p className="text-sm text-on-surface-variant">{t("common.loading")}</p>
      </div>
    )
  }

  if (slots.length === 0) {
    return (
      <div className="mx-auto max-w-7xl px-4 py-8 md:px-8 md:py-12">
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
    <div className="mx-auto max-w-7xl space-y-8 px-4 py-8 md:px-8 md:py-12">
      <PageHeader
        eyebrow={t("planner.week_label", {
          week: week_.week_number,
          year: week_.year,
        })}
        title={t("planner.title")}
        actions={
          <div className="flex items-center gap-2">
            {aiSettings?.enabled && <FillEmptySlotsButton weekId={week_.id} />}
            <Button
              onClick={() => setShoppingOpen(true)}
              className="gradient-primary editorial-shadow border-0 text-on-primary hover:opacity-90"
            >
              <Download className="mr-1.5 size-4" />
              {t("shopping.button")}
            </Button>
          </div>
        }
      />

      {aiFill && aiFill.weekId === week_.id && !aiFill.dismissed && (
        <RevertBanner
          count={aiFill.aiFilledPlateIds.length}
          onRevert={handleRevert}
          onDismiss={dismissAiFillBanner}
        />
      )}

      <div
        className="flex flex-wrap items-center justify-between gap-4 rounded-2xl bg-surface-container-low/60 px-4 py-3"
        data-testid="planner-toolbar"
      >
        <WeekNavigator
          year={week_.year}
          weekNumber={week_.week_number}
          onPrev={() => setYearWeek(shiftWeek(year, week, -1))}
          onNext={() => setYearWeek(shiftWeek(year, week, 1))}
          onCopy={handleCopy}
        />
        <div className="flex flex-wrap items-center gap-3">
          {dailyAvgKcal !== null && (
            <div className="flex items-baseline gap-2 rounded-full bg-surface-container-highest px-4 py-1.5">
              <span className="font-heading text-sm font-bold text-primary">
                {dailyAvgKcal.toLocaleString()} kcal
              </span>
              <span className="text-[9px] font-bold tracking-widest text-on-surface-variant uppercase">
                {t("planner.daily_avg")}
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
        </div>
      </div>

      <div className="-mx-2 hidden md:-mx-4 md:block">
        <PlannerGrid week={week_} slots={slots} />
      </div>
      <div className="md:hidden">
        <MobilePlannerGrid week={week_} slots={slots} />
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
