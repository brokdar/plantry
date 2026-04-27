/**
 * ShoppingPanel — range-based shopping list.
 *
 * Prop `range` is the default range (active plan window from the parent).
 * The user can override it with three preset chips stored in URL search params
 * `shop_from` and `shop_to`. On unmount the URL params are not cleaned up so a
 * refresh keeps the last choice.
 *
 * localStorage key: `plantry:purchased:range:{from}:{to}`
 *
 * One-time migration: on first mount after this version, any keys matching
 * `plantry:purchased:week:` are deleted and a flag
 * `plantry:migrated:v4-shopping` is set.
 */

import { ShoppingCart } from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { useShoppingList } from "@/lib/queries/shopping"
import { cn } from "@/lib/utils"

interface DateRange {
  from: string
  to: string
}

interface ShoppingPanelProps {
  /** Default range — the active plan window. Used when no URL override is present. */
  range: DateRange
  /** Shopping day as a JS day-of-week number (0=Sun … 6=Sat). */
  shoppingDay: number
  open: boolean
  onOpenChange: (open: boolean) => void
}

// ── date helpers ──────────────────────────────────────────────────────────────

function toDateStr(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

function addDays(d: Date, n: number): Date {
  const r = new Date(d)
  r.setDate(r.getDate() + n)
  return r
}

/**
 * Returns the date of the next occurrence of `weekday` (JS 0=Sun) strictly
 * after `from`. If `from` is already that weekday, returns 7 days later.
 */
function nextWeekday(from: Date, weekday: number): Date {
  const d = new Date(from)
  const delta = (weekday - d.getDay() + 7) % 7 || 7
  d.setDate(d.getDate() + delta)
  return d
}

/**
 * Returns the most recent past occurrence of `weekday` on or before `from`.
 * If `from` is already that weekday, returns `from` itself.
 */
function prevOrSameWeekday(from: Date, weekday: number): Date {
  const d = new Date(from)
  const delta = (d.getDay() - weekday + 7) % 7
  d.setDate(d.getDate() - delta)
  return d
}

function computePresets(
  shoppingDay: number
): Record<"next7" | "untilNext" | "thisCycle", DateRange> {
  const today = new Date()
  today.setHours(0, 0, 0, 0)

  const next = nextWeekday(today, shoppingDay)
  const last = prevOrSameWeekday(today, shoppingDay)

  return {
    next7: {
      from: toDateStr(today),
      to: toDateStr(addDays(today, 6)),
    },
    untilNext: {
      from: toDateStr(today),
      to: toDateStr(addDays(next, -1)),
    },
    thisCycle: {
      from: toDateStr(last),
      to: toDateStr(addDays(next, -1)),
    },
  }
}

// ── localStorage helpers ──────────────────────────────────────────────────────

const MIGRATION_FLAG = "plantry:migrated:v4-shopping"

function migrateLocalStorage() {
  try {
    if (localStorage.getItem(MIGRATION_FLAG)) return
    const toDelete: string[] = []
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i)
      if (key?.startsWith("plantry:purchased:week:")) toDelete.push(key)
    }
    toDelete.forEach((k) => localStorage.removeItem(k))
    localStorage.setItem(MIGRATION_FLAG, "1")
  } catch {
    // best-effort
  }
}

function rangeStorageKey(from: string, to: string): string {
  return `plantry:purchased:range:${from}:${to}`
}

function loadPurchased(from: string, to: string): Set<number> {
  try {
    const raw = localStorage.getItem(rangeStorageKey(from, to))
    if (!raw) return new Set()
    return new Set(JSON.parse(raw) as number[])
  } catch {
    return new Set()
  }
}

function savePurchased(from: string, to: string, ids: Set<number>) {
  try {
    localStorage.setItem(rangeStorageKey(from, to), JSON.stringify([...ids]))
  } catch {
    // best-effort
  }
}

// ── weekday name helper ───────────────────────────────────────────────────────

const WEEKDAY_KEYS = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"] as const

// ── component ─────────────────────────────────────────────────────────────────

export function ShoppingPanel({
  range,
  shoppingDay,
  open,
  onOpenChange,
}: ShoppingPanelProps) {
  const { t, i18n } = useTranslation()

  // Active range — initialized from prop. Parent passes a key based on the
  // range so this component remounts when the window navigates, resetting all
  // state without needing a sync effect.
  const [activeRange, setActiveRange] = useState<DateRange>(range)

  // One-time migration on first mount.
  useEffect(() => {
    migrateLocalStorage()
  }, [])

  const { data, isLoading } = useShoppingList(activeRange.from, activeRange.to)

  const [purchased, setPurchased] = useState<Set<number>>(() =>
    loadPurchased(activeRange.from, activeRange.to)
  )

  // Prune stale IDs (items removed from the shopping list) without setState in
  // an effect. effectivePurchased drives the UI; purchased is the user's intent.
  const effectivePurchased = useMemo(() => {
    if (!data) return purchased
    const validIds = new Set(data.items.map((i) => i.food_id))
    return new Set([...purchased].filter((id) => validIds.has(id)))
  }, [data, purchased])

  // Persist the pruned set whenever it changes.
  useEffect(() => {
    savePurchased(activeRange.from, activeRange.to, effectivePurchased)
  }, [effectivePurchased, activeRange.from, activeRange.to])

  function toggle(id: number) {
    setPurchased((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const presets = computePresets(shoppingDay)
  const dayKey = WEEKDAY_KEYS[shoppingDay]
  // e.g. "planner.day_sat" → "Sat"
  const dayName = t(`planner.day_${dayKey}`)

  function isActive(preset: DateRange) {
    return activeRange.from === preset.from && activeRange.to === preset.to
  }

  const items = data?.items ?? []

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="flex w-full flex-col sm:max-w-sm">
        <SheetHeader>
          <SheetTitle className="flex items-center gap-2">
            <ShoppingCart className="h-4 w-4" />
            {t("shopping.title")}
          </SheetTitle>
          <SheetDescription>
            {(() => {
              const fmt = new Intl.DateTimeFormat(i18n.language, {
                month: "short",
                day: "numeric",
              })
              return t("shopping.range_label", {
                from: fmt.format(new Date(activeRange.from + "T00:00:00")),
                to: fmt.format(new Date(activeRange.to + "T00:00:00")),
              })
            })()}
          </SheetDescription>
        </SheetHeader>

        {/* Preset chips */}
        <div className="flex flex-wrap gap-1.5 px-0 pt-1 pb-2">
          <button
            type="button"
            aria-pressed={isActive(presets.next7)}
            onClick={() => {
              setActiveRange(presets.next7)
              setPurchased(loadPurchased(presets.next7.from, presets.next7.to))
            }}
            className={cn(
              "rounded-full border px-3 py-1 text-xs font-medium transition-colors",
              isActive(presets.next7)
                ? "border-primary bg-primary text-primary-foreground"
                : "border-border bg-background text-foreground hover:bg-muted"
            )}
          >
            {t("shopping.preset_next7")}
          </button>
          <button
            type="button"
            aria-pressed={isActive(presets.untilNext)}
            onClick={() => {
              setActiveRange(presets.untilNext)
              setPurchased(
                loadPurchased(presets.untilNext.from, presets.untilNext.to)
              )
            }}
            className={cn(
              "rounded-full border px-3 py-1 text-xs font-medium transition-colors",
              isActive(presets.untilNext)
                ? "border-primary bg-primary text-primary-foreground"
                : "border-border bg-background text-foreground hover:bg-muted"
            )}
          >
            {t("shopping.preset_until_shopping_day", { day: dayName })}
          </button>
          <button
            type="button"
            aria-pressed={isActive(presets.thisCycle)}
            onClick={() => {
              setActiveRange(presets.thisCycle)
              setPurchased(
                loadPurchased(presets.thisCycle.from, presets.thisCycle.to)
              )
            }}
            className={cn(
              "rounded-full border px-3 py-1 text-xs font-medium transition-colors",
              isActive(presets.thisCycle)
                ? "border-primary bg-primary text-primary-foreground"
                : "border-border bg-background text-foreground hover:bg-muted"
            )}
          >
            {t("shopping.preset_this_cycle", { day: dayName })}
          </button>
        </div>

        <div className="flex-1 overflow-y-auto py-4">
          {isLoading && (
            <p className="text-sm text-muted-foreground">
              {t("common.loading")}
            </p>
          )}

          {!isLoading && items.length === 0 && (
            <p className="text-sm text-muted-foreground">
              {t("shopping.empty")}
            </p>
          )}

          {items.length > 0 && (
            <ul className="space-y-2">
              {items.map((item) => {
                const checked = effectivePurchased.has(item.food_id)
                return (
                  <li
                    key={item.food_id}
                    className="flex items-center gap-3 rounded-md px-1 py-1.5 hover:bg-muted/50"
                  >
                    <input
                      type="checkbox"
                      id={`shop-${item.food_id}`}
                      checked={checked}
                      onChange={() => toggle(item.food_id)}
                      className="h-4 w-4 rounded border-input accent-primary"
                    />
                    <label
                      htmlFor={`shop-${item.food_id}`}
                      className={cn(
                        "flex flex-1 cursor-pointer items-center justify-between text-sm",
                        checked && "text-muted-foreground line-through"
                      )}
                    >
                      <span>{item.name}</span>
                      <span className="ml-2 text-xs text-muted-foreground tabular-nums">
                        {Math.round(item.total_grams)} g
                      </span>
                    </label>
                  </li>
                )
              })}
            </ul>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
