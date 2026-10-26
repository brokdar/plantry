/** Plan decision D20: `/settings/$section`. */
export const SETTINGS_SECTIONS = [
  "household",
  "meals",
  "cooking",
  "assistant",
  "system",
] as const

export type SettingsSection = (typeof SETTINGS_SECTIONS)[number]

export const DEFAULT_SETTINGS_SECTION: SettingsSection = "household"

/** Legacy `?tab=` values (Phase 0) → sections. */
export const LEGACY_SETTINGS_TABS: Record<string, SettingsSection> = {
  general: "household",
  nutrition: "household",
  plan: "cooking",
  meal_slots: "meals",
  ai: "assistant",
  system: "system",
}

export function isSettingsSection(v: unknown): v is SettingsSection {
  return SETTINGS_SECTIONS.includes(v as SettingsSection)
}
