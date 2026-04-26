import { getISOWeek, getISOWeekYear } from "date-fns"
import { BarChart2, Download, Settings, Sparkles, Trash2 } from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { createFileRoute, Link } from "@tanstack/react-router"

import { ChatPanel } from "@/components/chat/ChatPanel"
import { PageHeader } from "@/components/editorial/PageHeader"
import { DateRangeNavigator } from "@/components/planner/DateRangeNavigator"
import { FillEmptySlotsButton } from "@/components/planner/FillEmptySlotsButton"
import { MobilePlannerGrid } from "@/components/planner/MobilePlannerGrid"
import { NutritionWeekSummary } from "@/components/planner/NutritionWeekSummary"
import { PlannerGrid, type PlannerDay } from "@/components/planner/PlannerGrid"
import { RevertBanner } from "@/components/planner/RevertBanner"
import { ShoppingPanel } from "@/components/planner/ShoppingPanel"
import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { deletePlate } from "@/lib/api/plates"
import { clearWeekPlates } from "@/lib/api/weeks"
import {
  computeAnchor,
  windowRange,
  type AnchorMode,
} from "@/lib/planner-window"
import { useAISettings } from "@/lib/queries/ai"
import { queryClient } from "@/lib/query-client"
import { plateKeys } from "@/lib/queries/keys"
import { usePlatesRange } from "@/lib/queries/plates"
import { useSettings } from "@/lib/queries/settings"
import { useTimeSlots } from "@/lib/queries/slots"
import { useWeekByDate, useWeekNutrition } from "@/lib/queries/weeks"
import { useChatUI } from "@/lib/stores/chat-ui"
import { usePlannerUI } from "@/lib/stores/planner-ui"
import { toast, toastError } from "@/lib/toast"

export const Route = createFileRoute("/")({
  validateSearch: (search): { date?: string } => ({
    date: (search.date as string) ?? undefined,
  }),
  component: PlanPage,
})

function PlanPage() {
  const { t, i18n } = useTranslation()
  const { date: dateParam } = Route.useSearch()
  // If ?date= is provided, compute initial offset so the window starts at that date.
  const [windowOffset, setWindowOffset] = useState(() => {
    if (!dateParam) return 0
    const target = new Date(dateParam + "T00:00:00")
    const today = new Date()
    const diff = Math.round(
      (target.getTime() - today.getTime()) / (1000 * 60 * 60 * 24)
    )
    // Round to nearest 7-day boundary (floor toward past)
    return Math.floor(diff / 7) * 7
  })
  const [shoppingOpen, setShoppingOpen] = useState(false)
  const [nutritionOpen, setNutritionOpen] = useState(false)
  const openChat = useChatUI((s) => s.setOpen)

  const settingsQuery = useSettings()
  const settingValue = (key: string, fallback: string) =>
    settingsQuery.data?.items.find((i) => i.key === key)?.value ?? fallback

  const anchorMode = settingValue("plan.anchor", "today") as AnchorMode
  const shoppingDay = Number(settingValue("plan.shopping_day", "1"))
  const weekStartsOn = settingValue("plan.week_starts_on", "monday") as
    | "monday"
    | "sunday"
    | "saturday"

  // Derive anchor fresh each render (pure derivation — no state)
  const anchor = computeAnchor({ mode: anchorMode, shoppingDay, weekStartsOn })
  const shifted = new Date(anchor)
  shifted.setDate(anchor.getDate() + windowOffset)
  const { from, to } = windowRange(shifted, 7)

  const slotsQuery = useTimeSlots(true)
  const platesQuery = usePlatesRange(from, to)
  const { data: aiSettings } = useAISettings()

  const plates = useMemo(
    () => platesQuery.data?.plates ?? [],
    [platesQuery.data]
  )

  const days: PlannerDay[] = useMemo(() => {
    const result: PlannerDay[] = []
    const start = new Date(from + "T00:00:00")
    for (let i = 0; i < 7; i++) {
      const d = new Date(start)
      d.setDate(start.getDate() + i)
      const dateStr = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
      const weekday = (d.getDay() + 6) % 7 // 0=Mon…6=Sun
      result.push({
        date: dateStr,
        weekday,
        plates: plates.filter((p) => p.date === dateStr),
      })
    }
    return result
  }, [from, plates])

  // TODO(phase-4): remove once ShoppingPanel and NutritionWeekSummary are range-based
  const anchorDate = new Date(from + "T00:00:00")
  const syntheticWeekQuery = useWeekByDate(
    getISOWeekYear(anchorDate),
    getISOWeek(anchorDate)
  )
  const syntheticWeekId = syntheticWeekQuery.data?.id ?? 0

  const nutritionQuery = useWeekNutrition(syntheticWeekId)

  const aiFill = usePlannerUI((s) => s.aiFill)
  const recordAiFilledPlate = usePlannerUI((s) => s.recordAiFilledPlate)
  const dismissAiFillBanner = usePlannerUI((s) => s.dismissAiFillBanner)
  const endAiFillSession = usePlannerUI((s) => s.endAiFillSession)

  // Watch plates created after the fill session started. Zustand actions don't
  // trigger re-render loops, so calling recordAiFilledPlate inside an effect is safe.
  useEffect(() => {
    if (
      !aiFill.startedAt ||
      aiFill.range?.from !== from ||
      aiFill.range?.to !== to
    )
      return
    for (const p of plates) {
      const created = Date.parse(p.created_at)
      if (!Number.isNaN(created) && created >= aiFill.startedAt) {
        recordAiFilledPlate(p.id)
      }
    }
  }, [aiFill, plates, from, to, recordAiFilledPlate])

  async function handleRevert() {
    for (const id of aiFill.plateIds) {
      await deletePlate(id)
    }
    endAiFillSession()
    await queryClient.invalidateQueries({ queryKey: plateKeys.range(from, to) })
  }

  function handleClearWindow() {
    if (!plates.length) return
    const snapshot = plates
    queryClient.setQueryData(plateKeys.range(from, to), { plates: [] })
    const timeoutId = setTimeout(async () => {
      try {
        await clearWeekPlates(syntheticWeekId)
        void queryClient.invalidateQueries({
          queryKey: plateKeys.range(from, to),
        })
      } catch (err) {
        toastError(err, t)
        void queryClient.invalidateQueries({
          queryKey: plateKeys.range(from, to),
        })
      }
    }, 5000)
    toast(t("planner.week_cleared"), {
      action: {
        label: t("common.undo"),
        onClick: () => {
          clearTimeout(timeoutId)
          queryClient.setQueryData(plateKeys.range(from, to), {
            plates: snapshot,
          })
        },
      },
      duration: 5000,
    })
  }

  const slots = slotsQuery.data?.items ?? []

  if (slotsQuery.isLoading || platesQuery.isLoading) {
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

  const fmt = new Intl.DateTimeFormat(i18n.language, {
    month: "short",
    day: "numeric",
  })
  const rangeLabel = t("planner.range_label", {
    from: fmt.format(new Date(from + "T00:00:00")),
    to: fmt.format(new Date(to + "T00:00:00")),
  })

  const showRevertBanner =
    aiFill.range?.from === from &&
    aiFill.range?.to === to &&
    !aiFill.dismissed &&
    aiFill.plateIds.length > 0

  const dailyAvgKcal = (() => {
    const days_ = nutritionQuery.data?.days
    if (!days_?.length) return null
    const plannedDays = days_.filter((d) => d.macros.kcal > 0)
    if (!plannedDays.length) return null
    const avg =
      plannedDays.reduce((acc, d) => acc + d.macros.kcal, 0) /
      plannedDays.length
    return Math.round(avg)
  })()

  return (
    <div className="mx-auto max-w-7xl space-y-8 px-4 py-8 md:px-8 md:py-12">
      <PageHeader
        eyebrow={rangeLabel}
        title={t("planner.title")}
        actions={
          <div className="flex items-center gap-2">
            {aiSettings?.enabled && (
              <FillEmptySlotsButton weekId={0} rangeFrom={from} rangeTo={to} />
            )}
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

      {showRevertBanner && (
        <RevertBanner
          count={aiFill.plateIds.length}
          onRevert={handleRevert}
          onDismiss={dismissAiFillBanner}
        />
      )}

      <div
        className="flex flex-wrap items-center justify-between gap-4 rounded-2xl bg-surface-container-low/60 px-4 py-3"
        data-testid="planner-toolbar"
      >
        <DateRangeNavigator
          from={from}
          to={to}
          days={7}
          planAnchor={anchorMode}
          shoppingDay={shoppingDay}
          onPrev={() => setWindowOffset((o) => o - 7)}
          onNext={() => setWindowOffset((o) => o + 7)}
          onToday={() => setWindowOffset(0)}
        />
        <TooltipProvider>
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
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={handleClearWindow}
                  aria-label={t("planner.clear_week")}
                  data-testid="clear-week"
                  className="hover:bg-destructive/10 hover:text-destructive [&_svg]:transition-transform [&_svg]:duration-150 hover:[&_svg]:scale-110"
                >
                  <Trash2 className="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent side="bottom">
                {t("planner.clear_week")}
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => setNutritionOpen(true)}
                  aria-label={t("nutrition.button")}
                  className="hover:bg-primary/10 hover:text-primary [&_svg]:transition-transform [&_svg]:duration-150 hover:[&_svg]:scale-110"
                >
                  <BarChart2 className="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent side="bottom">
                {t("nutrition.button")}
              </TooltipContent>
            </Tooltip>
            {aiSettings?.enabled && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => openChat(true)}
                    aria-label={t("chat.button")}
                    data-testid="chat-open-button"
                    className="hover:bg-primary/10 hover:text-primary [&_svg]:transition-transform [&_svg]:duration-150 hover:[&_svg]:scale-110"
                  >
                    <Sparkles className="size-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="bottom">
                  {t("chat.button")}
                </TooltipContent>
              </Tooltip>
            )}
          </div>
        </TooltipProvider>
      </div>

      <div className="-mx-2 hidden md:-mx-4 md:block">
        <PlannerGrid days={days} slots={slots} rangeFrom={from} rangeTo={to} />
      </div>
      <div className="md:hidden">
        <MobilePlannerGrid
          days={days}
          slots={slots}
          rangeFrom={from}
          rangeTo={to}
        />
      </div>

      <ShoppingPanel
        key={syntheticWeekId}
        weekId={syntheticWeekId}
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
          <NutritionWeekSummary weekId={syntheticWeekId} />
        </SheetContent>
      </Sheet>

      <ChatPanel weekId={syntheticWeekId} />
    </div>
  )
}
