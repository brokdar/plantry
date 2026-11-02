import type { SettingItem } from "@/lib/api/settings"

/**
 * Discovery needs a configured AI provider: the same rule as the assistant
 * entry points (`ai.provider` has a value). Pass `useSettingItems().data`;
 * while it is still loading this is false, so gate on `isSuccess` where a
 * flash of "unavailable" matters.
 */
export function discoveryAvailable(
  settings: Record<string, SettingItem> | undefined
): boolean {
  return Boolean(settings?.["ai.provider"]?.value)
}
