import { Link } from "@tanstack/react-router"
import { ChevronLeft, ChevronRight } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import type { RunSummary } from "@/lib/api/runs"
import { formatNumber } from "@/lib/plan/format"
import { useRuns } from "@/lib/queries/runs"

import { formatCost, formatDateTime, modelText, scoreText } from "./format"

/** The run log, 50 runs a page, newest first. A row opens its detail. */
export function RunsTable({ onOpen }: { onOpen: (id: number) => void }) {
  const { t } = useTranslation()
  // Cursors of the pages before the current one; the last is the current.
  const [cursors, setCursors] = useState<(number | null)[]>([null])
  const cursor = cursors[cursors.length - 1]
  const runs = useRuns({ cursor })

  if (runs.isPending) {
    return (
      <div className="flex flex-col gap-2" aria-busy="true">
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-9 w-full" />
      </div>
    )
  }
  if (runs.isError) {
    return (
      <div className="flex flex-wrap items-center gap-3 text-body text-ink">
        {t("settings_page.insights.runs_error")}
        <Button variant="outline" onClick={() => runs.refetch()}>
          {t("common.retry")}
        </Button>
      </div>
    )
  }

  const { items, next_cursor } = runs.data
  if (items.length === 0 && cursors.length === 1) {
    return (
      <div className="flex flex-col items-start gap-3 rounded-cell border border-line bg-panel p-5">
        <p className="text-body text-ink">
          {t("settings_page.insights.runs_empty")}
        </p>
        <Button asChild variant="outline">
          <Link to="/plan">
            {t("settings_page.insights.runs_empty_action")}
          </Link>
        </Button>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="overflow-x-auto rounded-cell border border-line bg-panel">
        <table className="w-full min-w-[56rem] text-left text-small text-ink">
          <caption className="sr-only">
            {t("settings_page.insights.runs_title")}
          </caption>
          <thead className="border-b border-line text-ink-muted">
            <tr>
              <Th>{t("settings_page.insights.col.date")}</Th>
              <Th>{t("settings_page.insights.col.kind")}</Th>
              <Th>{t("settings_page.insights.col.model")}</Th>
              <Th>{t("settings_page.insights.col.prompt_version")}</Th>
              <Th numeric>{t("settings_page.insights.col.tokens")}</Th>
              <Th numeric>{t("settings_page.insights.col.cost")}</Th>
              <Th>{t("settings_page.insights.col.status")}</Th>
              <Th>{t("settings_page.insights.col.score")}</Th>
              <Th>{t("settings_page.insights.col.outcome")}</Th>
            </tr>
          </thead>
          <tbody className="divide-y divide-line">
            {items.map((run) => (
              <RunRow key={run.id} run={run} onOpen={onOpen} />
            ))}
          </tbody>
        </table>
      </div>
      {(cursors.length > 1 || next_cursor !== null) && (
        <nav
          aria-label={t("settings_page.insights.pages_label")}
          className="flex justify-end gap-2"
        >
          <Button
            variant="outline"
            disabled={cursors.length === 1}
            onClick={() => setCursors((c) => c.slice(0, -1))}
          >
            <ChevronLeft strokeWidth={1.5} aria-hidden />
            {t("settings_page.insights.newer")}
          </Button>
          <Button
            variant="outline"
            disabled={next_cursor === null}
            onClick={() => setCursors((c) => [...c, next_cursor])}
          >
            {t("settings_page.insights.older")}
            <ChevronRight strokeWidth={1.5} aria-hidden />
          </Button>
        </nav>
      )}
    </div>
  )
}

function Th({
  children,
  numeric,
}: {
  children: React.ReactNode
  numeric?: boolean
}) {
  return (
    <th
      scope="col"
      className={`px-3 py-2 font-normal ${numeric ? "text-right" : ""}`}
    >
      {children}
    </th>
  )
}

function RunRow({
  run,
  onOpen,
}: {
  run: RunSummary
  onOpen: (id: number) => void
}) {
  const { t, i18n } = useTranslation()
  const tokens = run.tokens.input + run.tokens.output

  return (
    <tr
      className="cursor-pointer hover:bg-panel-sunken"
      onClick={() => onOpen(run.id)}
      data-testid={`run-row-${run.id}`}
    >
      <td className="px-3 py-2 whitespace-nowrap tabular-nums">
        <button
          type="button"
          className="rounded-cell text-left text-accent underline-offset-2 hover:underline"
          onClick={(e) => {
            e.stopPropagation()
            onOpen(run.id)
          }}
        >
          {formatDateTime(run.started_at, i18n)}
        </button>
      </td>
      <td className="px-3 py-2">
        {t(`settings_page.insights.kind.${run.kind}`)}
      </td>
      <td className="px-3 py-2">{modelText(run, i18n)}</td>
      <td className="px-3 py-2">{run.prompt_version ?? "–"}</td>
      <td className="px-3 py-2 text-right tabular-nums">
        {formatNumber(tokens, i18n)}
      </td>
      <td className="px-3 py-2 text-right whitespace-nowrap tabular-nums">
        {run.cost_estimate_usd === null ? (
          <span className="text-ink-muted">{formatCost(null, i18n)}</span>
        ) : (
          formatCost(run.cost_estimate_usd, i18n)
        )}
      </td>
      <td className="px-3 py-2">
        <span className={run.status === "ok" ? undefined : "text-warning"}>
          {t(`settings_page.insights.status.${run.status}`)}
        </span>
      </td>
      <td className="px-3 py-2 tabular-nums">{scoreText(run, i18n)}</td>
      <td className="px-3 py-2">
        {run.outcome ? t(`settings_page.insights.outcome.${run.outcome}`) : "–"}
      </td>
    </tr>
  )
}
