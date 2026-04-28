import { useMemo } from "react"
import { useTranslation } from "react-i18next"

import { SettingsCard } from "@/components/editorial/SettingsCard"
import { SettingRow } from "@/components/settings/SettingRow"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type { SettingItem } from "@/lib/api/settings"
import { useSetSetting, useSettings } from "@/lib/queries/settings"

const KEY_WEEK_STARTS_ON = "plan.week_starts_on"
const KEY_ANCHOR = "plan.anchor"
const KEY_SHOPPING_DAY = "plan.shopping_day"

type AnchorValue = "today" | "next_shopping_day" | "fixed_weekday"

// 2023-01-02 is a known Monday; offset i (0=Mon…6=Sun) gives the right date.
function localWeekdayName(i: number, locale: string): string {
  return new Intl.DateTimeFormat(locale, { weekday: "long" }).format(
    new Date(2023, 0, 2 + i)
  )
}

function indexByKey(items: SettingItem[]): Record<string, SettingItem> {
  return Object.fromEntries(items.map((it) => [it.key, it]))
}

export function PlanTab() {
  const { t, i18n } = useTranslation()
  const { data, isLoading } = useSettings()
  const setSetting = useSetSetting()

  // Locale-aware weekday names, 0=Monday…6=Sunday.
  const weekdayOptions = useMemo(
    () =>
      Array.from({ length: 7 }, (_, i) => ({
        value: String(i),
        label: localWeekdayName(i, i18n.language),
      })),
    [i18n.language]
  )

  // week_starts_on uses string values ("monday"|"sunday"|"saturday"); map to day index for display.
  const weekStartsOnOptions = useMemo(
    () => [
      { value: "monday", label: localWeekdayName(0, i18n.language) },
      { value: "sunday", label: localWeekdayName(6, i18n.language) },
      { value: "saturday", label: localWeekdayName(5, i18n.language) },
    ],
    [i18n.language]
  )

  const items = useMemo(() => indexByKey(data?.items ?? []), [data])

  if (isLoading || !data) {
    return (
      <SettingsCard title={t("settings_page.tabs.plan")}>
        <p className="text-sm text-on-surface-variant">{t("common.loading")}</p>
      </SettingsCard>
    )
  }

  const weekStartsOn = items[KEY_WEEK_STARTS_ON]?.value ?? "monday"
  const planAnchor = (items[KEY_ANCHOR]?.value ?? "today") as AnchorValue
  const shoppingDay = items[KEY_SHOPPING_DAY]?.value ?? "5"

  function handleWeekStartsOn(value: string) {
    setSetting.mutate({ key: KEY_WEEK_STARTS_ON, value })
  }

  function handleAnchor(value: AnchorValue) {
    setSetting.mutate({ key: KEY_ANCHOR, value })
  }

  function handleShoppingDay(value: string) {
    setSetting.mutate({ key: KEY_SHOPPING_DAY, value })
  }

  return (
    <div className="space-y-6">
      <SettingsCard title={t("settings_page.tabs.plan")}>
        <SettingRow
          label={t("settings.week_starts_on")}
          item={items[KEY_WEEK_STARTS_ON]}
        >
          <Select value={weekStartsOn} onValueChange={handleWeekStartsOn}>
            <SelectTrigger
              className="w-full"
              data-testid="plan-week-starts-on-select"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {weekStartsOnOptions.map(({ value, label }) => (
                <SelectItem key={value} value={value}>
                  {label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingRow>

        <SettingRow label={t("settings.plan_anchor")} item={items[KEY_ANCHOR]}>
          <fieldset className="space-y-3">
            <legend className="sr-only">{t("settings.plan_anchor")}</legend>
            {(
              [
                { value: "today", labelKey: "settings.plan_anchor_today" },
                {
                  value: "next_shopping_day",
                  labelKey: "settings.plan_anchor_next_shopping_day",
                },
                {
                  value: "fixed_weekday",
                  labelKey: "settings.plan_anchor_fixed_weekday",
                },
              ] as { value: AnchorValue; labelKey: string }[]
            ).map(({ value, labelKey }) => (
              <label
                key={value}
                className="flex cursor-pointer items-center gap-3"
                data-testid={`plan-anchor-option-${value}`}
              >
                <input
                  type="radio"
                  name="plan-anchor"
                  value={value}
                  checked={planAnchor === value}
                  onChange={() => handleAnchor(value)}
                  className="accent-primary"
                  data-testid={`plan-anchor-radio-${value}`}
                />
                <span className="text-sm text-on-surface">{t(labelKey)}</span>
              </label>
            ))}
          </fieldset>

          {planAnchor === "fixed_weekday" && (
            <div
              className="mt-3 space-y-1.5"
              data-testid="plan-fixed-weekday-picker"
            >
              <Label className="text-xs text-on-surface-variant">
                {t("settings.fixed_weekday_start")}
              </Label>
              <Select value={shoppingDay} onValueChange={handleShoppingDay}>
                <SelectTrigger
                  className="w-full"
                  data-testid="plan-fixed-weekday-select"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {weekdayOptions.map(({ value, label }) => (
                    <SelectItem key={value} value={value}>
                      {label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
        </SettingRow>

        <SettingRow
          label={t("settings.shopping_day")}
          item={items[KEY_SHOPPING_DAY]}
        >
          <Select value={shoppingDay} onValueChange={handleShoppingDay}>
            <SelectTrigger
              className="w-full"
              data-testid="plan-shopping-day-select"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {weekdayOptions.map(({ value, label }) => (
                <SelectItem key={value} value={value}>
                  {label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingRow>
      </SettingsCard>
    </div>
  )
}
