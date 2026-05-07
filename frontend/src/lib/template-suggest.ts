import { format, parseISO } from "date-fns"
import { de as deLocale } from "date-fns/locale"
import type { TFunction } from "i18next"

import { slotLabel } from "./slot-label"

const DAY_KEYS = [
  "planner.day_mon",
  "planner.day_tue",
  "planner.day_wed",
  "planner.day_thu",
  "planner.day_fri",
  "planner.day_sat",
  "planner.day_sun",
] as const

function dateLocale(language: string | undefined) {
  return language?.startsWith("de") ? deLocale : undefined
}

function shortDate(date: string, language: string | undefined): string {
  return format(parseISO(date), "MMM d", { locale: dateLocale(language) })
}

/** Auto-suggest a template name for the slot dialog, e.g. "Lunch · Mon". */
export function suggestSlotName(
  t: TFunction,
  _language: string | undefined,
  weekday: number,
  slotNameKey: string
): string {
  const dayKey = DAY_KEYS[weekday] ?? DAY_KEYS[0]
  return t("template.name_suggestion_slot", {
    slot: slotLabel(t, slotNameKey),
    day: t(dayKey),
    defaultValue: `${slotLabel(t, slotNameKey)} · ${t(dayKey)}`,
  })
}

/** Auto-suggest a template name for the day dialog, e.g. "Mon · Apr 28". */
export function suggestDayName(
  t: TFunction,
  language: string | undefined,
  date: string,
  weekday: number
): string {
  const dayKey = DAY_KEYS[weekday] ?? DAY_KEYS[0]
  return t("template.name_suggestion_day", {
    day: t(dayKey),
    date: shortDate(date, language),
    defaultValue: `${t(dayKey)} · ${shortDate(date, language)}`,
  })
}

/** Auto-suggest a template name for the week dialog, e.g. "Week · Apr 28". */
export function suggestWeekName(
  t: TFunction,
  language: string | undefined,
  startDate: string
): string {
  return t("template.name_suggestion_week", {
    date: shortDate(startDate, language),
    defaultValue: `Week · ${shortDate(startDate, language)}`,
  })
}
