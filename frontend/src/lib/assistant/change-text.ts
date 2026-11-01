import type { i18n as I18n, TFunction } from "i18next"

import type { Change } from "@/lib/domain/assistantEvents"
import { slotLabel } from "@/lib/slot-label"

/** Every `text_key` the backend notes (agent `noteChange`), each with a line
 *  under `assistant.change.<key>` in en and de. */
export const CHANGE_TEXT_KEYS = [
  "apply_preset",
  "set_session_portions",
  "set_plate_eaters",
  "set_availability_none",
  "set_availability_plan",
  "confirm",
  "unlock",
  "regenerate_window",
  "regenerate_day",
  "regenerate_plate",
  "regenerate_side",
  "regenerate_empty_cells",
  "create_plate",
  "add_food",
  "swap_food",
  "update_component",
  "remove_component",
  "delete_plate",
] as const

function weekday(date: unknown, language: string): string {
  if (typeof date !== "string") return ""
  const [y, m, d] = date.split("-").map(Number)
  if (!y || !m || !d) return ""
  return new Intl.DateTimeFormat(language, { weekday: "long" }).format(
    new Date(y, m - 1, d)
  )
}

/** A slot's name inside a sentence: lower-case, except in German where
 *  nouns keep their capital. */
export function slotInSentence(label: string, language: string): string {
  return language.startsWith("de") ? label : label.toLocaleLowerCase(language)
}

function text(v: unknown): string {
  return v === null || v === undefined ? "" : String(v)
}

/**
 * The change line's sentence (D16): `assistant.change.<text_key>` with the
 * change's params. Dates become weekdays, a slot key its display name.
 * `set_session_portions` and `set_plate_eaters` plural on the number; the
 * portions line adds the frozen part only when some are frozen.
 */
export function changeSentence(
  t: TFunction,
  i18n: Pick<I18n, "language">,
  change: Pick<Change, "text_key" | "params">
): string {
  const p = change.params
  let key = change.text_key
  const values: Record<string, string | number> = {
    name: text(p.name),
    from: text(p.from),
    day: weekday(p.date, i18n.language),
    slot:
      typeof p.slot === "string"
        ? slotInSentence(slotLabel(t, p.slot), i18n.language)
        : "",
    frozen: Number(p.frozen ?? 0),
    count: Number(
      key === "set_session_portions"
        ? p.portions
        : key === "set_plate_eaters"
          ? p.eaters
          : (p.count ?? 0)
    ),
  }
  if (key === "set_session_portions" && Number(p.frozen ?? 0) > 0) {
    key = "set_session_portions_frozen"
  }
  return t(`assistant.change.${key}`, {
    ...values,
    defaultValue: t("assistant.change.fallback"),
  })
}

/** Tools that only read: the status line says "Reading your plan…". */
const READ_TOOLS = new Set([
  "get_plan",
  "get_plates_range",
  "get_profile",
  "get_food",
  "get_preset",
  "list_foods",
  "list_presets",
  "list_slots",
])

/** The status line's key for the tool running (never the tool's own name). */
export function statusKey(tool: string): string {
  if (READ_TOOLS.has(tool)) return "assistant.status.reading"
  if (tool === "regenerate") return "assistant.status.regenerating"
  return "assistant.status.updating"
}
