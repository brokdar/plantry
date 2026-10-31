import { Link } from "@tanstack/react-router"
import { ChevronLeft } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import type { Insights, InsightsWeek } from "@/lib/api/insights"
import { formatDate, formatNumber, type Formatter } from "@/lib/plan/format"
import { useRunInsights } from "@/lib/queries/runs"

import { acceptanceRate, formatPercent, formatUSD } from "./format"
import { RunDetailSheet } from "./RunDetailSheet"
import { RunsTable } from "./RunsTable"
import { Sparkline, type SparkPoint } from "./Sparkline"
import { VersionTable } from "./VersionTable"

/**
 * Settings → Assistant → Insights (D37): weekly trends, the run log with a
 * detail sheet (`?run=`) and score components by prompt version.
 */
export function InsightsPage({
  runId,
  onRunChange,
}: {
  runId: number | undefined
  onRunChange: (id: number | undefined) => void
}) {
  const { t, i18n } = useTranslation()
  const insights = useRunInsights()

  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-8 px-4 py-6 md:px-8 md:py-10">
      <header className="flex flex-col gap-2">
        <Link
          to="/settings/$section"
          params={{ section: "assistant" }}
          className="-ml-2 inline-flex min-h-11 items-center gap-1 self-start rounded-lg px-2 text-body text-accent"
        >
          <ChevronLeft className="size-5" strokeWidth={1.5} aria-hidden />
          {t("settings_page.insights.back")}
        </Link>
        <h1 className="text-h1 font-semibold text-ink">
          {t("settings_page.insights.title")}
        </h1>
        <p className="max-w-prose text-body text-ink-muted">
          {t("settings_page.insights.summary")}
        </p>
      </header>

      <section
        aria-labelledby="insights-trends"
        className="flex flex-col gap-3"
      >
        <h2 id="insights-trends" className="text-h2 font-semibold text-ink">
          {t("settings_page.insights.trends_title")}
        </h2>
        {insights.data ? (
          <Trends data={insights.data} />
        ) : insights.isError ? (
          <div className="flex flex-wrap items-center gap-3 text-body text-ink">
            {t("settings_page.insights.trends_error")}
            <Button variant="outline" onClick={() => insights.refetch()}>
              {t("common.retry")}
            </Button>
          </div>
        ) : (
          <div className="grid gap-3 md:grid-cols-3" aria-busy="true">
            <Skeleton className="h-28" />
            <Skeleton className="h-28" />
            <Skeleton className="h-28" />
          </div>
        )}
      </section>

      <section aria-labelledby="insights-runs" className="flex flex-col gap-3">
        <h2 id="insights-runs" className="text-h2 font-semibold text-ink">
          {t("settings_page.insights.runs_title")}
        </h2>
        <RunsTable onOpen={onRunChange} />
      </section>

      {insights.data && (
        <section
          aria-labelledby="insights-versions"
          className="flex flex-col gap-3"
        >
          <h2 id="insights-versions" className="text-h2 font-semibold text-ink">
            {t("settings_page.insights.versions_title")}
          </h2>
          <VersionTable rows={insights.data.by_version} />
        </section>
      )}

      {insights.data && (
        <p className="text-small text-ink-muted">
          {t("settings_page.insights.prices_note", {
            date: formatDate(insights.data.prices_as_of, i18n, {
              dateStyle: "medium",
            }),
          })}
        </p>
      )}

      <RunDetailSheet runId={runId} onClose={() => onRunChange(undefined)} />
    </div>
  )
}

function weekLabel(week: InsightsWeek, f: Formatter): string {
  return f.t("settings_page.insights.week_of", {
    date: formatDate(week.start, f, { day: "numeric", month: "short" }),
  })
}

function points(
  weeks: InsightsWeek[],
  f: Formatter,
  value: (w: InsightsWeek) => number | null,
  text: (v: number) => string
): SparkPoint[] {
  return weeks.map((w) => {
    const v = value(w)
    return {
      key: w.start,
      label: weekLabel(w, f),
      value: v,
      text: v === null ? "" : text(v),
    }
  })
}

function latestText(pts: SparkPoint[], f: Formatter): string {
  const last = pts[pts.length - 1]
  return last && last.value !== null
    ? last.text
    : f.t("settings_page.insights.no_data")
}

function Trends({ data }: { data: Insights }) {
  const { t, i18n } = useTranslation()
  const { weeks } = data
  if (weeks.every((w) => w.runs === 0)) {
    return (
      <p className="text-body text-ink-muted">
        {t("settings_page.insights.trends_empty")}
      </p>
    )
  }

  const pct = (v: number) => formatPercent(v, i18n)
  const usd = (v: number) => formatUSD(v, i18n)
  const correction = points(weeks, i18n, (w) => w.correction_rate, pct)
  const acceptance = points(weeks, i18n, acceptanceRate, pct)
  const cost = points(weeks, i18n, (w) => w.cost_per_accepted, usd)
  const unpriced = weeks.reduce((n, w) => n + w.unpriced_runs, 0)
  const noData = t("settings_page.insights.no_data")

  return (
    <div className="flex flex-col gap-3">
      <div className="grid gap-3 md:grid-cols-3">
        <Sparkline
          title={t("settings_page.insights.correction_rate")}
          points={correction}
          latest={latestText(correction, i18n)}
          emptyText={noData}
          testId="spark-correction"
        />
        <Sparkline
          title={t("settings_page.insights.acceptance_rate")}
          points={acceptance}
          latest={latestText(acceptance, i18n)}
          emptyText={noData}
          testId="spark-acceptance"
        />
        <Sparkline
          title={t("settings_page.insights.cost_per_accepted")}
          points={cost}
          latest={latestText(cost, i18n)}
          emptyText={noData}
          note={
            unpriced > 0
              ? t("settings_page.insights.excludes_runs", { count: unpriced })
              : undefined
          }
          testId="spark-cost"
        />
      </div>
      <details className="text-small">
        <summary className="cursor-pointer text-ink-muted">
          {t("settings_page.insights.weeks_table")}
        </summary>
        <WeeksTable weeks={weeks} />
      </details>
    </div>
  )
}

/** The trends as a table: the sparklines' text equivalent. */
function WeeksTable({ weeks }: { weeks: InsightsWeek[] }) {
  const { t, i18n } = useTranslation()
  const pct = (v: number | null) => (v === null ? "–" : formatPercent(v, i18n))
  return (
    <div className="mt-2 overflow-x-auto rounded-cell border border-line bg-panel">
      <table className="w-full min-w-[40rem] text-left text-small text-ink">
        <thead className="border-b border-line text-ink-muted">
          <tr>
            <th scope="col" className="px-3 py-2 font-normal">
              {t("settings_page.insights.col.week")}
            </th>
            <th scope="col" className="px-3 py-2 text-right font-normal">
              {t("settings_page.insights.col.runs")}
            </th>
            <th scope="col" className="px-3 py-2 text-right font-normal">
              {t("settings_page.insights.acceptance_rate")}
            </th>
            <th scope="col" className="px-3 py-2 text-right font-normal">
              {t("settings_page.insights.correction_rate")}
            </th>
            <th scope="col" className="px-3 py-2 text-right font-normal">
              {t("settings_page.insights.col.cost")}
            </th>
            <th scope="col" className="px-3 py-2 text-right font-normal">
              {t("settings_page.insights.cost_per_accepted")}
            </th>
          </tr>
        </thead>
        <tbody className="divide-y divide-line">
          {weeks.map((w) => (
            <tr key={w.start} data-testid={`week-row-${w.start}`}>
              <th scope="row" className="px-3 py-2 font-normal">
                {weekLabel(w, i18n)}
              </th>
              <td className="px-3 py-2 text-right tabular-nums">
                {formatNumber(w.runs, i18n)}
              </td>
              <td className="px-3 py-2 text-right tabular-nums">
                {pct(acceptanceRate(w))}
              </td>
              <td className="px-3 py-2 text-right tabular-nums">
                {pct(w.correction_rate)}
              </td>
              <td className="px-3 py-2 text-right tabular-nums">
                {formatUSD(w.cost_usd, i18n)}
                {w.unpriced_runs > 0 && (
                  <span className="block text-caption text-ink-muted">
                    {t("settings_page.insights.excludes_runs", {
                      count: w.unpriced_runs,
                    })}
                  </span>
                )}
              </td>
              <td className="px-3 py-2 text-right tabular-nums">
                {w.cost_per_accepted === null
                  ? "–"
                  : formatUSD(w.cost_per_accepted, i18n)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
