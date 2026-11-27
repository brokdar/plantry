import type { SettingItem } from "@/lib/api/settings"

/**
 * chef.new_dishes_per_week as the planner reads it (THEME-6): a stored value
 * as is, else 1 with the Chef on and 0 with it off (the old explore default).
 */
export function newDishesPerWeek(
  items: Record<string, SettingItem> | undefined
): number {
  const item = items?.["chef.new_dishes_per_week"]
  if (
    !item ||
    (item.source === "default" && items?.["chef.enabled"]?.value !== "true")
  ) {
    return 0
  }
  return Number(item.value) || 0
}

/**
 * The Chef's planning parts (themes, New dishes per week) show only with
 * the Chef on and a provider (spec §11.1).
 */
export function chefPlanning(
  items: Record<string, SettingItem> | undefined
): boolean {
  return (
    items?.["chef.enabled"]?.value === "true" &&
    Boolean(items?.["ai.provider"]?.value)
  )
}
