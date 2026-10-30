import type { i18n as I18n } from "i18next"

import type { SessionLink } from "@/lib/api/sessions"
import { isCountUnit } from "@/lib/domain/units"

/** What the formatters need from i18next: `t` and the active language.
 *  Components pass `useTranslation().i18n`. */
export type Formatter = Pick<I18n, "t" | "language">

const FRACTIONS: ReadonlyArray<[number, string]> = [
  [1 / 4, "¼"],
  [1 / 3, "⅓"],
  [1 / 2, "½"],
  [2 / 3, "⅔"],
  [3 / 4, "¾"],
]
const FRACTION_TOLERANCE = 0.01

/** Units that are counted in halves and quarters rather than decimals. */
const FRACTION_UNITS = new Set(["tbsp", "tsp"])

function numberFormat(f: Formatter, maximumFractionDigits = 1) {
  return new Intl.NumberFormat(f.language, { maximumFractionDigits })
}

/** A plain number in the active locale, by default as a whole number
 *  ("1,840" / "1.840"). */
export function formatNumber(n: number, f: Formatter, maxFractionDigits = 0) {
  return numberFormat(f, maxFractionDigits).format(n)
}

/**
 * An amount with vulgar fractions for quarters and thirds ("1½", "⅔"), else
 * at most one decimal in the active locale. The backend has already rounded
 * (D26); this only formats.
 */
export function formatAmount(n: number, f: Formatter): string {
  const whole = Math.floor(n + FRACTION_TOLERANCE)
  const rest = n - whole
  if (rest > FRACTION_TOLERANCE) {
    const glyph = FRACTIONS.find(
      ([value]) => Math.abs(rest - value) < FRACTION_TOLERANCE
    )
    if (glyph) {
      return (whole > 0 ? numberFormat(f).format(whole) : "") + glyph[1]
    }
  }
  return numberFormat(f).format(n)
}

/**
 * An amount with its unit: "1½ pcs", "½ EL", "600 g". Count units and
 * spoons use fractions; mass and volume stay decimal. The short label comes
 * from `units.<unit>`, else the unit's name (`unit.<unit>.name`, plural-aware).
 */
export function formatQuantity(
  amount: number,
  unit: string,
  f: Formatter
): string {
  const value =
    isCountUnit(unit) || FRACTION_UNITS.has(unit)
      ? formatAmount(amount, f)
      : numberFormat(f).format(amount)
  const label = f.t(`units.${unit}`, {
    defaultValue: f.t(`unit.${unit}.name`, {
      count: amount,
      defaultValue: unit,
    }),
  })
  return `${value} ${label}`
}

/** Parses `YYYY-MM-DD` as a local calendar date (midnight, no UTC shift). */
function parseDate(iso: string): Date {
  const [y, m, d] = iso.split("-").map(Number)
  return new Date(y, m - 1, d)
}

/** A calendar date in the active locale, e.g. `{ weekday: "long", … }`. */
export function formatDate(
  iso: string,
  f: Formatter,
  options: Intl.DateTimeFormatOptions
): string {
  return new Intl.DateTimeFormat(f.language, options).format(parseDate(iso))
}

/** "Sun" / "So." */
export function weekdayShort(iso: string, f: Formatter): string {
  return formatDate(iso, f, { weekday: "short" })
}

/** "today", "tomorrow", "in 3 days", "3 days ago" (and the German forms),
 *  counted in calendar days. */
export function relativeDay(iso: string, today: string, f: Formatter): string {
  const days = (d: string) => {
    const [y, m, day] = d.split("-").map(Number)
    return Date.UTC(y, m - 1, day) / 86_400_000
  }
  return new Intl.RelativeTimeFormat(f.language, { numeric: "auto" }).format(
    days(iso) - days(today),
    "day"
  )
}

/** "Covers Sun dinner, Mon lunch, Mon dinner" for a session's links in
 *  order. `slotName` resolves a slot id to its display name. */
export function coverageText(
  links: readonly Pick<SessionLink, "date" | "slot_id">[],
  slotName: (slotId: number) => string,
  f: Formatter
): string {
  const cells = links.map((link) =>
    f.t("batch.cell", {
      weekday: weekdayShort(link.date, f),
      slot: slotName(link.slot_id),
    })
  )
  return f.t("batch.covers", { cells: cells.join(", ") })
}

/** What a batch badge says (D28). */
export type BatchBadgeContent =
  /** The cook cell: "Cook × 6". With `strip`, the cook strip's
   *  "Sun · Chili × 6". */
  | {
      variant: "cook"
      portions: number
      strip?: { weekday: string; name: string }
    }
  /** A linked cell: "2/3"; `frozen` adds "· frozen". */
  | { variant: "leftover" | "frozen"; position: number; meals: number }
  /** Cook strip extras: "2 spare", "2 to freeze". */
  | { variant: "surplus" | "toFreeze"; count: number }

/** The badge text for `content`, e.g. "Cook × 6", "2/3 · frozen". */
export function batchBadgeText(
  content: BatchBadgeContent,
  f: Formatter
): string {
  switch (content.variant) {
    case "cook": {
      const portions = formatAmount(content.portions, f)
      return content.strip
        ? f.t("batch.strip", { ...content.strip, portions })
        : f.t("batch.cook", { portions })
    }
    case "leftover":
    case "frozen":
      return f.t(
        content.variant === "frozen" ? "batch.frozen" : "batch.linked",
        { position: content.position, meals: content.meals }
      )
    case "surplus":
    case "toFreeze":
      return f.t(
        content.variant === "surplus" ? "batch.surplus" : "batch.to_freeze",
        { count: formatAmount(content.count, f) }
      )
  }
}
