import { createFileRoute, Link } from "@tanstack/react-router"
import { Eye, History } from "lucide-react"
import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import { CopyToCurrentButton } from "@/components/archive/CopyToCurrentButton"
import { FoodPlaceholder } from "@/components/editorial/FoodPlaceholder"
import { PageHeader } from "@/components/editorial/PageHeader"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { imageURL } from "@/lib/image-url"
import { useComponents } from "@/lib/queries/components"
import type { Component } from "@/lib/api/components"
import { useWeeksList } from "@/lib/queries/weeks"

export const Route = createFileRoute("/archive/")({
  component: ArchiveListPage,
})

const PAGE_SIZE = 20

function ArchiveListPage() {
  const { t } = useTranslation()
  const [offset, setOffset] = useState(0)
  const { data, isLoading } = useWeeksList(PAGE_SIZE, offset)
  const componentsQuery = useComponents({ limit: 500 })

  const componentsById = useMemo(() => {
    const map = new Map<number, Component>()
    for (const c of componentsQuery.data?.items ?? []) map.set(c.id, c)
    return map
  }, [componentsQuery.data])

  const items = data?.items ?? []
  const total = data?.total ?? 0
  const hasPrev = offset > 0
  const hasNext = offset + PAGE_SIZE < total

  function weekSummary(week: (typeof items)[number]) {
    const mealCount = week.plates.length
    const imagePaths: string[] = []
    const seen = new Set<string>()
    for (const plate of week.plates) {
      for (const pc of plate.components) {
        const c = componentsById.get(pc.component_id)
        if (c?.image_path && !seen.has(c.image_path)) {
          seen.add(c.image_path)
          const url = imageURL(c.image_path, c.updated_at)
          if (url) imagePaths.push(url)
          if (imagePaths.length >= 3) break
        }
      }
      if (imagePaths.length >= 3) break
    }
    return { mealCount, imagePaths }
  }

  return (
    <div
      className="mx-auto max-w-5xl space-y-8 px-4 py-8 md:px-8 md:py-12"
      data-testid="archive-list"
    >
      <PageHeader
        title={t("archive.title")}
        description={t("archive.subtitle")}
      />

      {isLoading && (
        <div className="flex flex-col gap-4">
          <Skeleton className="h-28 w-full rounded-2xl" />
          <Skeleton className="h-28 w-full rounded-2xl" />
          <Skeleton className="h-28 w-full rounded-2xl" />
        </div>
      )}

      {!isLoading && items.length === 0 && (
        <div className="editorial-shadow flex flex-col items-center gap-4 rounded-2xl bg-surface-container-lowest py-16 text-center">
          <div className="flex size-16 items-center justify-center rounded-full bg-surface-container-low text-primary">
            <History className="size-8" aria-hidden />
          </div>
          <p className="text-sm text-on-surface-variant">
            {t("archive.empty")}
          </p>
        </div>
      )}

      {!isLoading && items.length > 0 && (
        <ul className="flex flex-col gap-4">
          {items.map((week) => {
            const { mealCount, imagePaths } = weekSummary(week)
            const extra = Math.max(0, mealCount - imagePaths.length)
            return (
              <li key={week.id}>
                <div
                  className="editorial-shadow group relative flex flex-col items-start justify-between gap-6 overflow-hidden rounded-2xl bg-surface-container-lowest p-6 transition-all duration-300 hover:-translate-y-0.5 md:flex-row md:items-center"
                  data-testid={`archive-week-${week.id}`}
                >
                  <span className="absolute top-0 right-0 rounded-bl-xl bg-surface-container-high px-4 py-1 text-[10px] font-bold tracking-widest text-on-surface-variant uppercase">
                    {t("archive.read_only")}
                  </span>
                  <div className="flex-1 space-y-1">
                    <span className="text-sm font-bold text-primary">
                      {new Date(week.created_at).toLocaleDateString(undefined, {
                        month: "long",
                        year: "numeric",
                      })}
                    </span>
                    <h3 className="font-heading text-xl font-bold text-on-surface">
                      {t("archive.week_label", {
                        week: week.week_number,
                        year: week.year,
                      })}
                    </h3>
                    <p className="text-xs text-on-surface-variant">
                      {t("archive.meals_total", { count: mealCount })}
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    {imagePaths.map((src) => (
                      <img
                        key={src}
                        src={src}
                        alt=""
                        className="h-16 w-16 rounded-xl object-cover"
                      />
                    ))}
                    {imagePaths.length === 0 && mealCount > 0 && (
                      <FoodPlaceholder
                        category="default"
                        className="h-16 w-16"
                      />
                    )}
                    {extra > 0 && (
                      <span className="flex h-16 w-16 items-center justify-center rounded-xl bg-surface-container-low text-xs font-bold text-on-surface-variant">
                        +{extra}
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    <Button
                      asChild
                      variant="ghost"
                      size="sm"
                      data-testid={`review-${week.id}`}
                    >
                      <Link to="/archive/$id" params={{ id: String(week.id) }}>
                        <Eye className="mr-1 size-4" aria-hidden />
                        {t("archive.review")}
                      </Link>
                    </Button>
                    <CopyToCurrentButton
                      weekId={week.id}
                      size="sm"
                      testId={`copy-to-current-list-${week.id}`}
                    />
                  </div>
                </div>
              </li>
            )
          })}
        </ul>
      )}

      {!isLoading && items.length > 0 && !hasNext && (
        <div
          className="flex flex-col items-center gap-3 py-12 text-center"
          data-testid="archive-end-of-journey"
        >
          <div className="flex size-12 items-center justify-center rounded-full bg-surface-container-low text-on-surface-variant">
            <History className="size-5" aria-hidden />
          </div>
          <h3 className="font-heading text-lg font-bold text-on-surface">
            {t("archive.end_of_journey")}
          </h3>
          <p className="max-w-md text-sm text-on-surface-variant">
            {t("archive.end_of_journey_body")}
          </p>
        </div>
      )}

      {(hasPrev || hasNext) && (
        <div className="flex items-center justify-between">
          <Button
            variant="outline"
            size="sm"
            disabled={!hasPrev}
            onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
          >
            {t("common.previous")}
          </Button>
          <span className="text-xs text-on-surface-variant">
            {offset + 1}–{Math.min(offset + PAGE_SIZE, total)} / {total}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={!hasNext}
            onClick={() => setOffset(offset + PAGE_SIZE)}
          >
            {t("common.next")}
          </Button>
        </div>
      )}
    </div>
  )
}
