import { createFileRoute, Link } from "@tanstack/react-router"
import { Eye, History } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { CopyToCurrentButton } from "@/components/archive/CopyToCurrentButton"
import { PageHeader } from "@/components/editorial/PageHeader"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useWeeksList } from "@/lib/queries/weeks"

export const Route = createFileRoute("/archive/")({
  component: ArchiveListPage,
})

const PAGE_SIZE = 20

function ArchiveListPage() {
  const { t } = useTranslation()
  const [offset, setOffset] = useState(0)
  const { data, isLoading } = useWeeksList(PAGE_SIZE, offset)

  const items = data?.items ?? []
  const total = data?.total ?? 0
  const hasPrev = offset > 0
  const hasNext = offset + PAGE_SIZE < total

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
          {items.map((week) => (
            <li key={week.id}>
              <Link
                to="/archive/$id"
                params={{ id: String(week.id) }}
                className="editorial-shadow group relative flex flex-col items-start justify-between gap-6 overflow-hidden rounded-2xl bg-surface-container-lowest p-6 transition-all duration-300 hover:-translate-y-1 md:flex-row md:items-center"
                data-testid={`archive-week-${week.id}`}
              >
                <div className="absolute top-0 right-0 rounded-bl-xl bg-surface-container-highest px-4 py-1 text-[10px] font-bold tracking-widest text-on-surface-variant uppercase">
                  {t("archive.read_only", {
                    defaultValue: "Read only",
                  })}
                </div>
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
                </div>
                <div className="flex items-center gap-3">
                  <CopyToCurrentButton
                    weekId={week.id}
                    size="sm"
                    testId={`copy-to-current-list-${week.id}`}
                  />
                  <div className="flex items-center gap-2 text-xs font-bold text-primary">
                    <Eye className="size-4" aria-hidden />
                    {t("archive.review")}
                  </div>
                </div>
              </Link>
            </li>
          ))}
        </ul>
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
