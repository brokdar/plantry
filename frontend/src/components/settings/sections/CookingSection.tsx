import { TriangleAlert } from "lucide-react"
import { useMemo } from "react"
import { useTranslation } from "react-i18next"

import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { useSettingItems } from "@/lib/queries/settings"

import { NumberField, SettingsGroup, Stepper, Switch } from "../controls"
import { useSettingField, type SettingField } from "../setting-field"
import { SettingRow } from "../SettingRow"
import { SectionSkeleton } from "./HouseholdSection"

type AnchorValue = "today" | "next_shopping_day"

const ANCHORS: AnchorValue[] = ["today", "next_shopping_day"]

/** Leftovers past this many days get the food-safety warning. */
const SAFE_KEEP_DAYS = 4

// 2023-01-02 is a known Monday; offset i (0=Mon…6=Sun) gives the right date.
function localWeekdayName(i: number, locale: string): string {
  return new Intl.DateTimeFormat(locale, { weekday: "long" }).format(
    new Date(2023, 0, 2 + i)
  )
}

export function CookingSection() {
  const { data } = useSettingItems()
  if (!data) return <SectionSkeleton />
  return <CookingFields />
}

function CookingFields() {
  const { t, i18n } = useTranslation()

  const occasions = useSettingField("plan.max_cook_occasions_per_day", "1")
  const batch = useSettingField("plan.batch_cooking", "true")
  const keep = useSettingField("plan.default_keep_days", "3")
  const freezing = useSettingField("plan.freezing_enabled", "false")
  const freezerKeep = useSettingField("plan.freezer_keep_days", "90")
  const variety = useSettingField("plan.variety_weeks", "3")
  const explore = useSettingField("plan.explore_per_week", "0")
  const shopping = useSettingField("plan.shopping_day", "0")
  const anchor = useSettingField("plan.anchor", "next_shopping_day")

  // Locale-aware weekday names, 0=Monday…6=Sunday.
  const weekdayOptions = useMemo(
    () =>
      Array.from({ length: 7 }, (_, i) => ({
        value: String(i),
        label: localWeekdayName(i, i18n.language),
      })),
    [i18n.language]
  )

  const days = (count: number) => t("settings_page.days", { count })
  const weeks = (count: number) => t("settings_page.weeks", { count })

  const keepDays = Number(keep.value)
  const shoppingDayName = localWeekdayName(
    Number(shopping.value),
    i18n.language
  )
  const planAnchor = anchor.value as AnchorValue
  const batchOn = batch.value === "true"
  const freezingOn = freezing.value === "true"

  return (
    <div className="flex flex-col gap-10">
      <SettingsGroup title={t("settings_page.cooking.cooking_group")}>
        <StepperRow
          field={occasions}
          label={t("settings_page.cooking.occasions_label")}
          effectKey="settings_page.cooking.occasions_effect"
          min={1}
          max={3}
        />
        <SettingRow
          label={t("settings_page.cooking.batch_label")}
          description={t(
            batchOn
              ? "settings_page.cooking.batch_effect_on"
              : "settings_page.cooking.batch_effect_off"
          )}
          item={batch.item}
          onClearOverride={batch.clear}
          saved={batch.saved}
          htmlFor="setting-batch-cooking"
        >
          <Switch
            id="setting-batch-cooking"
            checked={batchOn}
            onChange={(next) => batch.save(String(next))}
          />
        </SettingRow>
      </SettingsGroup>

      <SettingsGroup title={t("settings_page.cooking.leftovers_group")}>
        <StepperRow
          field={keep}
          label={t("settings_page.cooking.keep_label")}
          effectKey="settings_page.cooking.keep_effect"
          min={1}
          max={7}
          format={days}
        >
          {keepDays > SAFE_KEEP_DAYS && (
            <p
              role="status"
              className="flex items-start gap-2 text-small text-ink"
              data-testid="keep-days-warning"
            >
              <TriangleAlert
                className="mt-0.5 size-4 shrink-0 text-warning"
                strokeWidth={1.5}
                aria-hidden
              />
              {t("settings_page.cooking.keep_warning")}
            </p>
          )}
        </StepperRow>
        <SettingRow
          label={t("settings_page.cooking.freezing_label")}
          description={t(
            freezingOn
              ? "settings_page.cooking.freezing_effect_on"
              : "settings_page.cooking.freezing_effect_off"
          )}
          item={freezing.item}
          onClearOverride={freezing.clear}
          saved={freezing.saved}
          htmlFor="setting-freezing"
        >
          <Switch
            id="setting-freezing"
            checked={freezingOn}
            onChange={(next) => freezing.save(String(next))}
          />
        </SettingRow>
        {freezingOn && (
          <SettingRow
            label={t("settings_page.cooking.freezer_keep_label")}
            description={t("settings_page.cooking.freezer_keep_effect", {
              count: Number(freezerKeep.value),
            })}
            item={freezerKeep.item}
            onClearOverride={freezerKeep.clear}
            saved={freezerKeep.saved}
            htmlFor="setting-freezer-keep"
          >
            <NumberField
              id="setting-freezer-keep"
              value={Number(freezerKeep.value)}
              min={1}
              max={365}
              suffix={t("settings_page.days_unit", {
                count: Number(freezerKeep.value),
              })}
              onCommit={(n) => freezerKeep.save(String(n))}
              testId="freezer-keep-input"
            />
          </SettingRow>
        )}
      </SettingsGroup>

      <SettingsGroup title={t("settings_page.cooking.variety_group")}>
        <StepperRow
          field={variety}
          label={t("settings_page.cooking.variety_label")}
          effectKey="settings_page.cooking.variety_effect"
          min={0}
          max={8}
          format={weeks}
        />
        <StepperRow
          field={explore}
          label={t("settings_page.cooking.explore_label")}
          effectKey="settings_page.cooking.explore_effect"
          min={0}
          max={3}
        />
      </SettingsGroup>

      <SettingsGroup title={t("settings_page.cooking.calendar_group")}>
        <SettingRow
          label={t("settings.shopping_day")}
          description={t("settings_page.cooking.shopping_effect", {
            day: shoppingDayName,
          })}
          item={shopping.item}
          onClearOverride={shopping.clear}
          saved={shopping.saved}
        >
          <WeekdaySelect
            value={shopping.value}
            onChange={shopping.save}
            options={weekdayOptions}
            testId="plan-shopping-day-select"
            label={t("settings.shopping_day")}
          />
        </SettingRow>

        <SettingRow
          label={t("settings.plan_anchor")}
          description={t(`settings_page.cooking.anchor_effect.${planAnchor}`, {
            day: shoppingDayName,
          })}
          item={anchor.item}
          onClearOverride={anchor.clear}
          saved={anchor.saved}
        >
          <fieldset className="flex flex-col gap-1">
            <legend className="sr-only">{t("settings.plan_anchor")}</legend>
            {ANCHORS.map((value) => (
              <label
                key={value}
                className="flex min-h-11 cursor-pointer items-center gap-3 md:min-h-9"
                data-testid={`plan-anchor-option-${value}`}
              >
                <input
                  type="radio"
                  name="plan-anchor"
                  value={value}
                  checked={planAnchor === value}
                  onChange={() => anchor.save(value)}
                  className="size-4 accent-accent"
                  data-testid={`plan-anchor-radio-${value}`}
                />
                <span className="text-body text-ink">
                  {t(`settings.plan_anchor_${value}`)}
                </span>
              </label>
            ))}
          </fieldset>
        </SettingRow>
      </SettingsGroup>
    </div>
  )
}

/** A setting edited with − / +, whose effect line takes the count. */
function StepperRow({
  field,
  label,
  effectKey,
  min,
  max,
  format,
  children,
}: {
  field: SettingField
  label: string
  effectKey: string
  min: number
  max: number
  format?: (value: number) => string
  children?: React.ReactNode
}) {
  const { t } = useTranslation()
  const value = Number(field.value)
  return (
    <SettingRow
      label={label}
      description={t(effectKey, { count: value })}
      item={field.item}
      onClearOverride={field.clear}
      saved={field.saved}
    >
      <Stepper
        label={label}
        value={value}
        min={min}
        max={max}
        format={format}
        onChange={(next) => field.save(String(next))}
      />
      {children}
    </SettingRow>
  )
}

function WeekdaySelect({
  value,
  onChange,
  options,
  testId,
  label,
}: {
  value: string
  onChange: (value: string) => void
  options: { value: string; label: string }[]
  testId: string
  label: string
}) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger
        className="w-full max-w-60 pointer-coarse:h-11"
        data-testid={testId}
        aria-label={label}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          {options.map((opt) => (
            <SelectItem key={opt.value} value={opt.value}>
              {opt.label}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  )
}
