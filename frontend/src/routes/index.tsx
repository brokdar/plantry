import {
  addWeeks,
  getISOWeek,
  getISOWeekYear,
  setISOWeek,
  setISOWeekYear,
} from "date-fns"
import { BarChart2, Settings, ShoppingCart, Sparkles } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { createFileRoute, Link } from "@tanstack/react-router"

import { ChatPanel } from "@/components/chat/ChatPanel"
import { PageHeader } from "@/components/editorial/PageHeader"
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
import { useCopyWeek, useWeekByDate } from "@/lib/queries/weeks"
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

  return (
    <div className="px-4 py-6 md:px-8 md:py-8">
      <PageHeader
        title={t("planner.title")}
        description={t("planner.week_label", {
          week: week_.week_number,
          year: week_.year,
        })}
        actions={
          <>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShoppingOpen(true)}
            >
              <ShoppingCart className="mr-1.5 size-4" />
              {t("shopping.button")}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setNutritionOpen(true)}
            >
              <BarChart2 className="mr-1.5 size-4" />
              {t("nutrition.button")}
            </Button>
            {aiSettings?.enabled && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => openChat(true)}
                data-testid="chat-open-button"
              >
                <Sparkles className="mr-1.5 size-4" />
                {t("chat.button")}
              </Button>
            )}
            <WeekNavigator
              year={week_.year}
              weekNumber={week_.week_number}
              onPrev={() => setYearWeek(shiftWeek(year, week, -1))}
              onNext={() => setYearWeek(shiftWeek(year, week, 1))}
              onCopy={handleCopy}
            />
          </>
        }
      />

      <div className="editorial-shadow overflow-hidden rounded-2xl bg-surface-container-lowest p-4 md:p-6">
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
