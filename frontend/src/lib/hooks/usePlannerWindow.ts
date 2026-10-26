import {
  computeAnchor,
  windowRange,
  type AnchorMode,
} from "@/lib/planner-window"
import { useSettings } from "@/lib/queries/settings"

/**
 * The 7-day plan window, anchored per the `plan.*` settings and shifted by
 * `offsetDays`. Shared by the planner (`/`) and the shop page (`/shop`).
 */
export function usePlannerWindow(offsetDays = 0): { from: string; to: string } {
  const settingsQuery = useSettings()
  const settingValue = (key: string, fallback: string) =>
    settingsQuery.data?.items.find((i) => i.key === key)?.value ?? fallback

  const anchorMode = settingValue("plan.anchor", "today") as AnchorMode
  const shoppingDay = Number(settingValue("plan.shopping_day", "0"))
  const weekStartsOn = settingValue("plan.week_starts_on", "monday") as
    | "monday"
    | "sunday"
    | "saturday"

  // Derive anchor fresh each render (pure derivation — no state)
  const anchor = computeAnchor({ mode: anchorMode, shoppingDay, weekStartsOn })
  const shifted = new Date(anchor)
  shifted.setDate(anchor.getDate() + offsetDays)
  return windowRange(shifted, 7)
}
