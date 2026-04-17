import { createFileRoute, Link } from "@tanstack/react-router"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
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
    <section className="flex flex-col gap-6" data-testid="archive-list">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          {t("archive.title")}
        </h1>
        <p className="text-sm text-muted-foreground">{t("archive.subtitle")}</p>
      </div>

      {isLoading && (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-14 w-full" />
          <Skeleton className="h-14 w-full" />
          <Skeleton className="h-14 w-full" />
        </div>
      )}

      {!isLoading && items.length === 0 && (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            {t("archive.empty")}
          </CardContent>
        </Card>
      )}

      {!isLoading && items.length > 0 && (
        <ul className="flex flex-col gap-2">
          {items.map((week) => (
            <li key={week.id}>
              <Link
                to="/archive/$id"
                params={{ id: String(week.id) }}
                className="flex items-center justify-between rounded-md border border-border bg-card px-4 py-3 text-sm transition-colors hover:bg-accent/40"
                data-testid={`archive-week-${week.id}`}
              >
                <span className="font-medium">
                  {t("archive.week_label", {
                    week: week.week_number,
                    year: week.year,
                  })}
                </span>
                <span className="text-xs text-muted-foreground">
                  {new Date(week.created_at).toLocaleDateString()}
                </span>
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
          <span className="text-xs text-muted-foreground">
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
    </section>
  )
}
