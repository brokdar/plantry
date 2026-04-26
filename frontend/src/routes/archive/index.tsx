import { createFileRoute, Link } from "@tanstack/react-router"
import { History } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { CopyToCurrentButton } from "@/components/archive/CopyToCurrentButton"
import type { PlannerDay } from "@/components/planner/PlannerGrid"
import { ReadOnlyPlannerGrid } from "@/components/planner/ReadOnlyPlannerGrid"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"
import { useTimeSlots } from "@/lib/queries/slots"
import { useCurrentWeek, useWeek, useWeeksList } from "@/lib/queries/weeks"
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

export const Route = createFileRoute("/archive/")({
  component: ArchiveListPage,
})

const PAGE_SIZE = 20

function ArchiveListPage() {
  const { t } = useTranslation()
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [loadedItems, setLoadedItems] = useState<Week[]>([])
  const [offset, setOffset] = useState(0)

  const { data, isLoading } = useWeeksList(PAGE_SIZE, offset)
  const { data: currentWeek } = useCurrentWeek()
  const allItems = [...loadedItems, ...(data?.items ?? [])].filter(
    (w) => w.id !== currentWeek?.id
  )
  const effectiveId = selectedId ?? allItems[0]?.id ?? null
  const hasMore =
    (data?.total ?? 0) > loadedItems.length + (data?.items?.length ?? 0)

  function handleLoadMore() {
    setLoadedItems((prev) => [...prev, ...(data?.items ?? [])])
    setOffset((prev) => prev + PAGE_SIZE)
  }

  return (
    <div className="flex items-start" data-testid="archive-list">
      {/* List panel — full-width scrolling page on mobile, sticky sidebar on desktop */}
      <div className="flex w-full flex-col pb-24 md:sticky md:top-16 md:h-[calc(100dvh-4rem)] md:w-72 md:shrink-0 md:overflow-y-auto md:border-r md:border-outline-variant/30 md:pb-0">
        {/* Mobile-only page header */}
        <div className="px-4 pt-8 pb-2 md:hidden">
          <h1 className="font-heading text-3xl font-extrabold tracking-tight text-on-surface">
            {t("archive.title")}
          </h1>
          <p className="mt-1 text-sm text-on-surface-variant">
            {t("archive.subtitle")}
          </p>
        </div>

        {/* Desktop-only sidebar header */}
        <div className="hidden flex-none border-b border-outline-variant/20 px-4 py-3 md:block">
          <span className="text-[10px] font-bold tracking-widest text-on-surface-variant uppercase">
            {t("archive.title")}
          </span>
        </div>

        <div className="flex-1 overflow-y-auto">
          {isLoading && loadedItems.length === 0 && (
            <div className="flex flex-col gap-1 p-2">
              <Skeleton className="h-14 w-full rounded-lg" />
              <Skeleton className="h-14 w-full rounded-lg" />
              <Skeleton className="h-14 w-full rounded-lg" />
            </div>
          )}

          {!isLoading && allItems.length === 0 && (
            <div className="flex flex-col items-center gap-4 px-4 py-16 text-center">
              <History
                className="size-8 text-on-surface-variant/40"
                aria-hidden
              />
              <p className="text-sm text-on-surface-variant">
                {t("archive.empty")}
              </p>
            </div>
          )}

          {allItems.length > 0 && (
            <WeekList
              items={allItems}
              effectiveId={effectiveId}
              onSelect={setSelectedId}
              t={t}
            />
          )}
        </div>

        {hasMore && (
          <div className="flex-none border-t border-outline-variant/20 p-3">
            <Button
              variant="ghost"
              size="sm"
              className="w-full text-xs"
              disabled={isLoading}
              onClick={handleLoadMore}
            >
              {t("archive.load_more")}
            </Button>
          </div>
        )}
      </div>

      {/* Right panel — desktop only */}
      <div className="hidden min-w-0 flex-1 flex-col md:flex">
        <RightPanel effectiveId={effectiveId} t={t} />
      </div>
    </div>
  )
}

function WeekList({
  items,
  effectiveId,
  onSelect,
  t,
}: {
  items: Week[]
  effectiveId: number | null
  onSelect: (id: number) => void
  t: ReturnType<typeof useTranslation>["t"]
}) {
  return (
    <>
      {items.map((week, i) => {
        const showYear = i === 0 || items[i - 1].year !== week.year
        return (
          <div key={week.id}>
            {showYear && (
              <div className="sticky top-0 z-10 bg-surface-container/90 px-4 py-1 text-[10px] font-bold tracking-widest text-on-surface-variant uppercase backdrop-blur-sm">
                {week.year}
              </div>
            )}
            <div
              className={cn(
                "flex items-center justify-between transition-colors hover:bg-surface-container",
                effectiveId === week.id ? "bg-primary-container/60" : ""
              )}
            >
              {/* Link: navigates on mobile, intercepts click on desktop */}
              <Link
                to="/archive/$id"
                params={{ id: String(week.id) }}
                data-testid={`archive-week-${week.id}`}
                onClick={(e) => {
                  if (window.matchMedia("(min-width: 768px)").matches) {
                    e.preventDefault()
                    onSelect(week.id)
                  }
                }}
                className="flex min-w-0 flex-1 flex-col gap-0.5 px-4 py-3"
              >
                <span className="truncate text-sm font-semibold text-on-surface">
                  {t("archive.week_label", {
                    week: week.week_number,
                    year: week.year,
                  })}
                </span>
                <span className="text-[11px] text-on-surface-variant">
                  {t("archive.meals_total", { count: week.plates.length })}
                </span>
              </Link>
              <div className="shrink-0 pr-2">
                <CopyToCurrentButton
                  weekId={week.id}
                  size="sm"
                  iconOnly
                  testId={`copy-to-current-list-${week.id}`}
                />
              </div>
            </div>
          </div>
        )
      })}
    </>
  )
}

function RightPanel({
  effectiveId,
  t,
}: {
  effectiveId: number | null
  t: ReturnType<typeof useTranslation>["t"]
}) {
  const slotsQuery = useTimeSlots(false)
  const weekQuery = useWeek(effectiveId ?? 0)
  const week = weekQuery.data
  const slots = slotsQuery.data?.items ?? []

  if (effectiveId === null) {
    return (
      <div className="flex h-96 flex-col items-center justify-center gap-4 text-center">
        <History className="size-10 text-on-surface-variant/30" aria-hidden />
        <p className="text-sm text-on-surface-variant">
          {t("archive.select_week")}
        </p>
      </div>
    )
  }

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-b border-outline-variant/30 px-6 py-4">
        {week ? (
          <h1 className="font-heading text-xl font-bold text-on-surface">
            {t("archive.week_label", {
              week: week.week_number,
              year: week.year,
            })}
          </h1>
        ) : (
          <Skeleton className="h-7 w-40" />
        )}
        {week && (
          <CopyToCurrentButton
            weekId={week.id}
            testId={`copy-to-current-split-${week.id}`}
          />
        )}
      </div>

      {(weekQuery.isLoading || slotsQuery.isLoading) && (
        <Skeleton className="m-6 h-64 w-full rounded-2xl" />
      )}

      {week && slots.length > 0 && (
        <div className="overflow-x-auto p-6" data-testid="archive-detail">
          <ReadOnlyPlannerGrid days={weekToPlannerDays(week)} slots={slots} />
        </div>
      )}
    </>
  )
}
