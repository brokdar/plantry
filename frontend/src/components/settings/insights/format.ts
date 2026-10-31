import type { InsightsWeek } from "@/lib/api/insights"
import type { RunSummary } from "@/lib/api/runs"
import type { Formatter } from "@/lib/plan/format"

/** An estimated cost in USD; small amounts keep four decimals. */
export function formatUSD(usd: number, f: Formatter): string {
  return new Intl.NumberFormat(f.language, {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: usd !== 0 && Math.abs(usd) < 0.01 ? 4 : 2,
  }).format(usd)
}

/** A run's cost, or "cost unknown" for a model without a price. */
export function formatCost(usd: number | null, f: Formatter): string {
  return usd === null
    ? f.t("settings_page.insights.cost_unknown")
    : formatUSD(usd, f)
}

/** A 0–1 share as a whole percentage. */
export function formatPercent(share: number, f: Formatter): string {
  return new Intl.NumberFormat(f.language, {
    style: "percent",
    maximumFractionDigits: 0,
  }).format(share)
}

/** A run's start time, e.g. "30 Oct 2026, 18:00". */
export function formatDateTime(iso: string, f: Formatter): string {
  return new Intl.DateTimeFormat(f.language, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(iso))
}

/** Accepted share of the plans that were decided (accepted or discarded);
 *  null when none were. */
export function acceptanceRate(week: InsightsWeek): number | null {
  const decided = week.accepted + week.discarded
  return decided === 0 ? null : week.accepted / decided
}

/** The model, or "Heuristic" for a run planned without the assistant. */
export function modelText(run: RunSummary, f: Formatter): string {
  if (run.model) return run.model
  return run.chooser === "heuristic"
    ? f.t("settings_page.insights.heuristic")
    : "–"
}

/** "100 % covered · 1 violation", or "–" for an unscored run. */
export function scoreText(run: RunSummary, f: Formatter): string {
  if (!run.score) return "–"
  const violations = Object.values(run.score.hard_violations).reduce(
    (a, b) => a + b,
    0
  )
  return f.t("settings_page.insights.score_summary", {
    coverage: formatPercent(run.score.coverage, f),
    count: violations,
  })
}
