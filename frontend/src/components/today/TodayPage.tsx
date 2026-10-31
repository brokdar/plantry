import { Link } from "@tanstack/react-router"
import { useMemo } from "react"
import { useTranslation } from "react-i18next"

import { BatchHighlightProvider } from "@/components/batch/BatchHighlightProvider"
import { Button } from "@/components/ui/button"
import type { Today } from "@/lib/api/today"
import {
  buildTodayHeader,
  buildTodayPlan,
  type TodayPlanView,
} from "@/lib/plan/today"
import { usePlan } from "@/lib/queries/plan"
import { useToday } from "@/lib/queries/today"
import { cn } from "@/lib/utils"

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
 * alone renders the verdict, the rating question and the banners at once;
 * `GET /api/plan` for the same window then fills in the cards, rows,
 * reminders and the week strip with batch colours and badges.
 */
export function TodayPage({ date, today }: TodayPageProps) {
  const { t } = useTranslation()
  const query = useToday(date, today)
  if (query.isPending) {
    return (
      <Frame>
        <div
          role="status"
          aria-label={t("today.loading")}
          className="flex max-w-xl flex-col gap-6"
          data-testid="today-skeleton"
        >
          <div className="flex flex-col gap-2">
            <Block className="h-4 w-40" />
            <Block className="h-8 w-64" />
          </div>
          <PlanSkeleton />
        </div>
      </Frame>
    )
  }
  if (query.isError) {
    return (
      <Frame>
        <LoadError retry={query.refetch} />
      </Frame>
    )
  }
  return <TodayLoaded data={query.data} today={today} />
}

function TodayLoaded({ data, today }: { data: Today; today: string }) {
  const { i18n } = useTranslation()
  const header = useMemo(
    () => buildTodayHeader(data, today, i18n),
    [data, today, i18n]
  )
  const plan = usePlan(data.window_start)
  const view = useMemo(
    () => (plan.data ? buildTodayPlan(data, plan.data, today, i18n) : null),
    [data, plan.data, today, i18n]
  )
  // A day with a cook or leftover card leads with it; the setup nag follows.
  const busyDay =
    data.cook.length > 0 ||
    data.meals.some((m) => m.link !== null && m.link.kind !== "fresh")

  return (
    <Frame>
      <BatchHighlightProvider>
        <div className="grid gap-8 md:grid-cols-[minmax(0,1fr)_17rem] lg:grid-cols-[minmax(0,1fr)_20rem] lg:gap-12">
          <div className="flex min-w-0 flex-col gap-6">
            <Verdict
              date={data.date}
              today={today}
              dateLine={header.dateLine}
              headline={header.headline}
            />
            {header.noPlan && <NoPlanCard windowStart={data.window_start} />}
            {header.draftOnly && (
              <DraftOnlyCard windowStart={data.window_start} />
            )}
            {!busyDay && <RecipesCard missing={header.recipesMissing} />}
            {view ? (
              <DayPlan view={view} />
            ) : plan.isError ? (
              <LoadError retry={plan.refetch} />
            ) : (
              <PlanSkeleton />
            )}
            {busyDay && <RecipesCard missing={header.recipesMissing} />}
            <RateQuestion rate={header.rate} />
            {header.planNext && (
              <PlanBanner nextStart={header.planNext} today={today} />
            )}
          </div>
          <aside className="hidden md:block">
            {view ? (
              <WeekStrip
                board={view.board}
                windowStart={data.window_start}
                date={data.date}
              />
            ) : (
              !plan.isError && (
                <div className="flex flex-col gap-2" aria-hidden="true">
                  <Block className="h-4 w-24" />
                  {[0, 1, 2, 3, 4, 5, 6].map((i) => (
                    <Block key={i} className="h-11" />
                  ))}
                </div>
              )
            )}
          </aside>
        </div>
      </BatchHighlightProvider>
    </Frame>
  )
}

/** Below the recipe target, "Add N more recipes". */
function RecipesCard({ missing }: { missing: number }) {
  const { t } = useTranslation()
  if (missing <= 0) return null
  return (
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
          {t("today.setup.title", { count: missing })}
        </h2>
        <p className="text-small text-ink-muted">{t("today.setup.body")}</p>
      </div>
      <Button asChild className="h-11 px-4 md:pointer-fine:h-9">
        <Link to="/recipes">{t("today.setup.action")}</Link>
      </Button>
    </section>
  )
}

/** On an unplanned window, "Plan this week". */
function NoPlanCard({ windowStart }: { windowStart: string }) {
  const { t } = useTranslation()
  return (
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
  )
}

/** On a window that holds only drafts, "Review plan": Today reads confirmed
 *  plates only, so the draft has to be accepted on the plan first. */
function DraftOnlyCard({ windowStart }: { windowStart: string }) {
  const { t } = useTranslation()
  return (
    <section
      className="flex flex-col items-start gap-3 rounded-lg border border-line bg-panel p-4"
      data-testid="today-draft-only"
    >
      <p className="text-body text-ink">{t("today.draft_only.body")}</p>
      <Button asChild className="h-11 px-4 md:pointer-fine:h-9">
        <Link to="/plan" search={{ start: windowStart }}>
          {t("today.draft_only.action")}
        </Link>
      </Button>
    </section>
  )
}

/** The main card (a cook card per batch cooked that day, else the leftover
 *  card), one row per meal slot, and the reminders. */
function DayPlan({ view }: { view: TodayPlanView }) {
  const { t } = useTranslation()
  return (
    <>
      {view.cook.map((card) => (
        <CookCard key={card.session.id} card={card} />
      ))}
      {view.leftover && <LeftoverCard card={view.leftover} />}
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

function Block({ className }: { className: string }) {
  return (
    <div
      className={cn("animate-pulse rounded-cell bg-panel-sunken", className)}
    />
  )
}

/** Skeletons in place of the main card and the rows (UX §6). */
function PlanSkeleton() {
  return (
    <div
      className="flex flex-col gap-6"
      aria-hidden="true"
      data-testid="today-plan-skeleton"
    >
      <Block className="h-32 rounded-lg" />
      <div className="flex flex-col gap-2">
        {[0, 1, 2].map((i) => (
          <Block key={i} className="h-10" />
        ))}
      </div>
    </div>
  )
}

function LoadError({ retry }: { retry: () => unknown }) {
  const { t } = useTranslation()
  return (
    <div role="alert" className="flex flex-col items-start gap-3">
      <p className="text-body text-ink">{t("today.error")}</p>
      <Button variant="outline" onClick={() => void retry()}>
        {t("common.retry")}
      </Button>
    </div>
  )
}
