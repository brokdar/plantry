import { useTranslation } from "react-i18next"

import { useSettingField } from "@/components/settings/setting-field"
import { formatNumber } from "@/lib/plan/format"
import { useChefInsights } from "@/lib/queries/tips"

import { formatPercent, formatUSD } from "./format"

/** The Chef block (§10, first metrics): tips shown, applied and dismissed
 *  over the last eight weeks, and this month's capped spend by kind. Only
 *  while the Chef is on. */
export function ChefBlock() {
  const on = useSettingField("chef.enabled", "false").value === "true"
  if (!on) return null
  return <ChefBlockContent />
}

function ChefBlockContent() {
  const { t, i18n } = useTranslation()
  const { data } = useChefInsights()
  if (!data) return null
  const { shown, applied, dismissed } = data.tips
  const rows: [label: string, value: string, testId: string][] = [
    [
      t("settings_page.insights.chef.shown"),
      formatNumber(shown, i18n),
      "chef-tips-shown",
    ],
    [
      t("settings_page.insights.chef.applied"),
      formatNumber(applied, i18n),
      "chef-tips-applied",
    ],
    [
      t("settings_page.insights.chef.dismissed"),
      formatNumber(dismissed, i18n),
      "chef-tips-dismissed",
    ],
    [
      t("settings_page.insights.chef.apply_rate"),
      shown === 0 ? "–" : formatPercent(applied / shown, i18n),
      "chef-tips-rate",
    ],
    ...data.spend.map((s): [string, string, string] => [
      t(`settings_page.insights.chef.kind.${s.kind}`),
      formatUSD(s.usd, i18n),
      `chef-spend-${s.kind}`,
    ]),
  ]
  return (
    <section
      aria-labelledby="insights-chef"
      className="flex flex-col gap-3"
      data-testid="insights-chef"
    >
      <h2 id="insights-chef" className="text-h2 font-semibold text-ink">
        {t("settings_page.insights.chef.title")}
      </h2>
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-6 gap-y-1.5 self-start text-small">
        {rows.map(([label, value, testId]) => (
          <div key={testId} data-testid={testId} className="contents">
            <dt className="text-ink-muted">{label}</dt>
            <dd className="text-ink tabular-nums">{value}</dd>
          </div>
        ))}
      </dl>
    </section>
  )
}
