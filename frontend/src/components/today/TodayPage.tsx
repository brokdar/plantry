import { Link } from "@tanstack/react-router"
import { useMemo } from "react"
import { useTranslation } from "react-i18next"

import { BatchHighlightProvider } from "@/components/batch/BatchHighlightProvider"
import { Button } from "@/components/ui/button"
import type { Today } from "@/lib/api/today"
import { buildToday, type TodayView } from "@/lib/plan/today"
import { usePlan } from "@/lib/queries/plan"
import { useToday } from "@/lib/queries/today"

import { CookCard, LeftoverCard } from "./CookCard"
import { MealRow } from "./MealRow"
import { PlanBanner } from "./PlanBanner"
import { RateQuestion } from "./RateQuestion"
import { Reminders } from "./Reminders"
import { Verdict } from "./Verdict"
import { WeekStrip } from "./WeekStrip"

interface TodayPageProps {
  /** The day to show (YYYY-MM-DD). */
  date: string
  /** The real today. */
  today: string
}

/**
 * `/today` (UX §5.2): what to do today in one glance. `GET /api/today`
 * gives the verdict, cooking, reminders, the rating question and the
 * banners; `GET /api/plan` for the same window adds batch colours, badges
 * and the week strip.
 */
export function TodayPage({ date, today }: TodayPageProps) {
  const { t } = useTranslation()
  const query = useToday(date)
  if (query.isPending) return <TodaySkeleton />
  if (query.isError) {
    return <LoadError message={t("today.error")} retry={query.refetch} />
  }
  return <TodayWithPlan data={query.data} today={today} />
}

function TodayWithPlan({ data, today }: { data: Today; today: string }) {
  const { t, i18n } = useTranslation()
  const plan = usePlan(data.window_start)
  const view = useMemo(
    () => (plan.data ? buildToday(data, plan.data, i18n) : null),
    [data, plan.data, i18n]
  )
  if (plan.isError) {
    return <LoadError message={t("today.error")} retry={plan.refetch} />
  }
  if (!view) return <TodaySkeleton />

  return (
    <Frame>
      <BatchHighlightProvider>
        <div className="grid gap-8 md:grid-cols-[minmax(0,1fr)_17rem] lg:grid-cols-[minmax(0,1fr)_20rem] lg:gap-12">
          <div className="flex min-w-0 flex-col gap-6">
            <Verdict
              date={data.date}
              today={today}
              dateLine={view.dateLine}
              headline={view.headline}
            />
            <MainCards view={view} windowStart={data.window_start} />
            {view.rows.length > 0 && (
              <section aria-label={t("today.meals")}>
                <ul className="flex flex-col">
                  {view.rows.map((row) => (
                    <MealRow key={row.cell.key} row={row} />
                  ))}
                </ul>
              </section>
            )}
            <Reminders reminders={view.reminders} />
            <RateQuestion rate={view.rate} />
            {view.planNext && <PlanBanner nextStart={view.planNext} />}
          </div>
          <aside className="hidden md:block">
            <WeekStrip
              board={view.board}
              windowStart={data.window_start}
              date={data.date}
            />
          </aside>
        </div>
      </BatchHighlightProvider>
    </Frame>
  )
}

/** The main card: a cook card per batch cooked today, else the leftover
 *  card; on an unplanned window "Plan this week". Below the recipe target
 *  the "Add N more recipes" card comes first. */
function MainCards({
  view,
  windowStart,
}: {
  view: TodayView
  windowStart: string
}) {
  const { t } = useTranslation()
  return (
    <>
      {view.recipesMissing > 0 && (
        <section
          aria-labelledby="today-setup"
          className="flex flex-col items-start gap-3 rounded-lg border border-line bg-panel p-4"
          data-testid="today-setup-card"
        >
          <div className="flex flex-col gap-0.5">
            <h2
              id="today-setup"
              className="font-sans text-body font-semibold text-ink"
            >
              {t("today.setup.title", { count: view.recipesMissing })}
            </h2>
            <p className="text-small text-ink-muted">{t("today.setup.body")}</p>
          </div>
          <Button asChild className="h-11 px-4 md:pointer-fine:h-9">
            <Link to="/components">{t("today.setup.action")}</Link>
          </Button>
        </section>
      )}
      {view.noPlan && view.recipesMissing === 0 && (
        <section
          className="flex flex-col items-start gap-3 rounded-lg border border-line bg-panel p-4"
          data-testid="today-no-plan"
        >
          <p className="text-body text-ink">{t("today.no_plan.body")}</p>
          <Button asChild className="h-11 px-4 md:pointer-fine:h-9">
            <Link to="/plan" search={{ start: windowStart }}>
              {t("today.no_plan.action")}
            </Link>
          </Button>
        </section>
      )}
      {view.cook.map((card) => (
        <CookCard key={card.session.id} card={card} />
      ))}
      {view.leftover && <LeftoverCard card={view.leftover} />}
    </>
  )
}

function Frame({ children }: { children: React.ReactNode }) {
  return (
    <div className="mx-auto w-full max-w-5xl px-4 py-6 md:px-8 md:py-8">
      {children}
    </div>
  )
}

function LoadError({
  message,
  retry,
}: {
  message: string
  retry: () => unknown
}) {
  const { t } = useTranslation()
  return (
    <Frame>
      <div role="alert" className="flex flex-col items-start gap-3">
        <p className="text-body text-ink">{message}</p>
        <Button variant="outline" onClick={() => void retry()}>
          {t("common.retry")}
        </Button>
      </div>
    </Frame>
  )
}

/** Skeletons in place of the header, the main card and the rows (UX §6). */
function TodaySkeleton() {
  const { t } = useTranslation()
  return (
    <Frame>
      <div
        role="status"
        aria-label={t("today.loading")}
        className="flex max-w-xl flex-col gap-6"
        data-testid="today-skeleton"
      >
        <div className="flex flex-col gap-2">
          <div className="h-4 w-40 animate-pulse rounded-cell bg-panel-sunken" />
          <div className="h-8 w-64 animate-pulse rounded-cell bg-panel-sunken" />
        </div>
        <div className="h-32 animate-pulse rounded-lg bg-panel-sunken" />
        <div className="flex flex-col gap-2">
          {[0, 1, 2].map((i) => (
            <div
              key={i}
              className="h-10 animate-pulse rounded-cell bg-panel-sunken"
            />
          ))}
        </div>
      </div>
    </Frame>
  )
}
