import { useTranslation } from "react-i18next"

import type { InsightsByVersion } from "@/lib/api/insights"
import { formatNumber, type Formatter } from "@/lib/plan/format"

import { formatPercent } from "./format"

/** The score components shown per version (D23), in order. Shares read as
 *  percentages, the rest as numbers. */
const COMPONENTS: { path: string; label: string; share: boolean }[] = [
  {
    path: "coverage",
    label: "settings_page.insights.score.coverage",
    share: true,
  },
  {
    path: "preference_fit",
    label: "settings_page.insights.score.preference_fit",
    share: true,
  },
  {
    path: "macro_deviation.weekly_pct",
    label: "settings_page.insights.score.macro_weekly",
    share: false,
  },
  {
    path: "variety.distinct",
    label: "settings_page.insights.score.variety_distinct",
    share: false,
  },
  {
    path: "waste_portions",
    label: "settings_page.insights.score.waste_portions",
    share: false,
  },
  {
    path: "unknown_data_share",
    label: "settings_page.insights.score.unknown_data",
    share: true,
  },
]

function componentText(
  value: number | undefined,
  share: boolean,
  f: Formatter
): string {
  if (value === undefined) return "–"
  return share ? formatPercent(value, f) : formatNumber(value, f, 1)
}

/** Score components and violations grouped by kind, prompt version and
 *  model, for the groups that have any. */
export function VersionTable({ groups }: { groups: InsightsByVersion[] }) {
  const { t, i18n } = useTranslation()
  // Chat turns carry no score or violations; only planning runs compare.
  const rows = groups.filter(
    (g) =>
      Object.keys(g.score_avg).length > 0 ||
      Object.keys(g.violations).length > 0
  )

  if (rows.length === 0) {
    return (
      <p className="text-body text-ink-muted">
        {t("settings_page.insights.versions_empty")}
      </p>
    )
  }

  return (
    <div className="overflow-x-auto rounded-cell border border-line bg-panel">
      <table className="w-full min-w-[56rem] text-left text-small text-ink">
        <caption className="sr-only">
          {t("settings_page.insights.versions_title")}
        </caption>
        <thead className="border-b border-line text-ink-muted">
          <tr>
            <th scope="col" className="px-3 py-2 font-normal">
              {t("settings_page.insights.col.kind")}
            </th>
            <th scope="col" className="px-3 py-2 font-normal">
              {t("settings_page.insights.col.prompt_version")}
            </th>
            <th scope="col" className="px-3 py-2 font-normal">
              {t("settings_page.insights.col.model")}
            </th>
            <th scope="col" className="px-3 py-2 text-right font-normal">
              {t("settings_page.insights.col.runs")}
            </th>
            {COMPONENTS.map((c) => (
              <th
                key={c.path}
                scope="col"
                className="px-3 py-2 text-right font-normal"
              >
                {t(c.label)}
              </th>
            ))}
            <th scope="col" className="px-3 py-2 font-normal">
              {t("settings_page.insights.col.violations")}
            </th>
          </tr>
        </thead>
        <tbody className="divide-y divide-line">
          {rows.map((row) => {
            const violations = Object.entries(row.violations).sort(([a], [b]) =>
              a.localeCompare(b)
            )
            return (
              <tr key={`${row.kind}|${row.prompt_version}|${row.model}`}>
                <td className="px-3 py-2">
                  {t(`settings_page.insights.kind.${row.kind}`)}
                </td>
                <td className="px-3 py-2">{row.prompt_version || "–"}</td>
                <td className="px-3 py-2">{row.model || "–"}</td>
                <td className="px-3 py-2 text-right tabular-nums">
                  {formatNumber(row.runs, i18n)}
                </td>
                {COMPONENTS.map((c) => (
                  <td
                    key={c.path}
                    className="px-3 py-2 text-right tabular-nums"
                  >
                    {componentText(row.score_avg[c.path], c.share, i18n)}
                  </td>
                ))}
                <td className="px-3 py-2 tabular-nums">
                  {violations.length === 0
                    ? t("settings_page.insights.violations_none")
                    : violations
                        .map(
                          ([rule, n]) => `${rule} × ${formatNumber(n, i18n)}`
                        )
                        .join(", ")}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
