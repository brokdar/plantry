import type { TFunction } from "i18next"

/**
 * Resolve a TimeSlot's display name.
 *
 * Preferred: the name_key is registered in the active i18n bundle (e.g. `slot.breakfast`).
 * Fallback: strip any `slot.` prefix, replace separators with spaces, title-case the result —
 * so a user-entered key like `slot.plan-nav-cd997ac8` renders as "Plan Nav Cd997ac8" rather
 * than leaking the raw key into the UI.
 */
export function slotLabel(t: TFunction, nameKey: string): string {
  const translated = t(nameKey, { defaultValue: "" })
  if (translated && translated !== nameKey) {
    return translated
  }

  const stripped = nameKey.replace(/^slot\./i, "")
  const words = stripped.split(/[-._\s]+/).filter(Boolean)
  if (words.length === 0) {
    return nameKey
  }
  return words
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase())
    .join(" ")
}
