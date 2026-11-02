import { useTranslation } from "react-i18next"

import type { InsightsDiscovery } from "@/lib/api/insights"
import { formatNumber } from "@/lib/plan/format"

import { formatPercent, formatUSD } from "./format"

/** The discovery runs of the range: ideas shown, recipes added, the share
 *  added and what an added recipe cost, web searches included. */
export function DiscoveryBlock({
  discovery: d,
}: {
  discovery: InsightsDiscovery
}) {
  const { t, i18n } = useTranslation()
  const rows: [label: string, value: string, testId: string][] = [
    ["runs", formatNumber(d.runs, i18n), "discovery-runs"],
    ["suggestions", formatNumber(d.suggestions, i18n), "discovery-suggestions"],
    ["accepted", formatNumber(d.accepted, i18n), "discovery-accepted"],
    [
      "acceptance_rate",
      d.acceptance_rate === null ? "–" : formatPercent(d.acceptance_rate, i18n),
      "discovery-rate",
    ],
    [
      "cost_per_accepted",
      d.cost_per_accepted_recipe === null
        ? "–"
        : formatUSD(d.cost_per_accepted_recipe, i18n),
      "discovery-cost",
    ],
  ]
  return (
    <section
      aria-labelledby="insights-discovery"
      className="flex flex-col gap-3"
      data-testid="insights-discovery"
    >
      <h2 id="insights-discovery" className="text-h2 font-semibold text-ink">
        {t("settings_page.insights.discovery.title")}
      </h2>
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-6 gap-y-1.5 self-start text-small">
        {rows.map(([key, value, testId]) => (
          <div key={key} data-testid={testId} className="contents">
            <dt className="text-ink-muted">
              {t(`settings_page.insights.discovery.${key}`)}
            </dt>
            <dd className="text-ink tabular-nums">{value}</dd>
          </div>
        ))}
      </dl>
    </section>
  )
}
