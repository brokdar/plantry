/**
 * ShoppingPanel — range-based shopping list.
 *
 * Prop `range` is the viewed plan window from the parent. Preset chips are
 * derived from it (plus today): "Rest of this window", "This window" and
 * "Next window".
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
import { shiftYMD, todayISO } from "@/lib/planner-window"
import { useShoppingList } from "@/lib/queries/shopping"
import { cn } from "@/lib/utils"

interface DateRange {
  from: string
  to: string
}

// ── presets ───────────────────────────────────────────────────────────────────

type PresetKey = "restOfWindow" | "thisWindow" | "nextWindow"

const PRESET_LABEL_KEYS: Record<PresetKey, string> = {
  restOfWindow: "shopping.preset_rest_of_window",
  thisWindow: "shopping.preset_this_window",
  nextWindow: "shopping.preset_next_window",
}

function windowLength(range: DateRange): number {
  const from = new Date(range.from + "T00:00:00")
  const to = new Date(range.to + "T00:00:00")
  return Math.round((to.getTime() - from.getTime()) / 86_400_000) + 1
}

/**
 * Presets derived from the viewed window and today (YYYY-MM-DD).
 * "Rest of this window" is present only when today lies inside the window
 * after its first day (on the first day it equals "This window").
 */
function computePresets(
  range: DateRange,
  today: string
): Partial<Record<PresetKey, DateRange>> {
  const presets: Partial<Record<PresetKey, DateRange>> = {}
  if (today > range.from && today <= range.to) {
    presets.restOfWindow = { from: today, to: range.to }
  }
  presets.thisWindow = range
  presets.nextWindow = {
    from: shiftYMD(range.to, 1),
    to: shiftYMD(range.to, windowLength(range)),
  }
  return presets
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

// ── state ─────────────────────────────────────────────────────────────────────

/**
 * Active range, presets and purchased state for one viewed window. The caller
 * keys its component on the range so this state resets when the window
 * navigates. ShoppingPanel holds it outside the Sheet so chip and tick state
 * survive closing the sheet.
 */
function useShoppingListState(range: DateRange) {
  const { i18n } = useTranslation()

  // Today is read once at mount so render stays pure.
  const [today] = useState(todayISO)
  const presets = computePresets(range, today)

  // Active range — defaults to the first preset ("Rest of this window" when
  // today is inside the window, else "This window").
  const [activeRange, setActiveRange] = useState<DateRange>(
    () => presets.restOfWindow ?? range
  )

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

  function selectPreset(preset: DateRange) {
    setActiveRange(preset)
    setPurchased(loadPurchased(preset.from, preset.to))
  }

  const fmt = new Intl.DateTimeFormat(i18n.language, {
    month: "short",
    day: "numeric",
  })
  const rangeLabel = {
    from: fmt.format(new Date(activeRange.from + "T00:00:00")),
    to: fmt.format(new Date(activeRange.to + "T00:00:00")),
  }

  return {
    presets,
    activeRange,
    selectPreset,
    rangeLabel,
    items: data?.items ?? [],
    isLoading,
    purchased: effectivePurchased,
    toggle,
  }
}

type ShoppingListState = ReturnType<typeof useShoppingListState>

// ── components ────────────────────────────────────────────────────────────────

interface ShoppingPanelProps {
  /** The viewed plan window. All presets are derived from it. */
  range: DateRange
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ShoppingPanel({
  range,
  open,
  onOpenChange,
}: ShoppingPanelProps) {
  const { t } = useTranslation()
  const state = useShoppingListState(range)

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="flex w-full flex-col sm:max-w-sm">
        <SheetHeader>
          <SheetTitle className="flex items-center gap-2">
            <ShoppingCart className="h-4 w-4" />
            {t("shopping.title")}
          </SheetTitle>
          <SheetDescription>
            {t("shopping.range_label", state.rangeLabel)}
          </SheetDescription>
        </SheetHeader>
        <ShoppingListBody state={state} />
      </SheetContent>
    </Sheet>
  )
}

/** The shopping list rendered inline on a page (`/shop`), no sheet. Key it on
 *  the range, like ShoppingPanel. */
export function ShoppingList({ range }: { range: DateRange }) {
  const { t } = useTranslation()
  const state = useShoppingListState(range)

  return (
    <div className="flex flex-col">
      <p className="text-sm text-ink-muted tabular-nums">
        {t("shopping.range_label", state.rangeLabel)}
      </p>
      <ShoppingListBody state={state} />
    </div>
  )
}

function ShoppingListBody({ state }: { state: ShoppingListState }) {
  const { t } = useTranslation()
  const { presets, activeRange, items, isLoading } = state

  function isActive(preset: DateRange) {
    return activeRange.from === preset.from && activeRange.to === preset.to
  }

  return (
    <>
      {/* Preset chips */}
      <div className="flex flex-wrap gap-1.5 px-0 pt-1 pb-2">
        {(Object.keys(PRESET_LABEL_KEYS) as PresetKey[]).map((key) => {
          const preset = presets[key]
          if (!preset) return null
          return (
            <button
              key={key}
              type="button"
              aria-pressed={isActive(preset)}
              onClick={() => state.selectPreset(preset)}
              className={cn(
                "rounded-full border px-3 py-1 text-xs font-medium transition-colors",
                isActive(preset)
                  ? "border-primary bg-primary text-primary-foreground"
                  : "border-border bg-background text-foreground hover:bg-muted"
              )}
            >
              {t(PRESET_LABEL_KEYS[key])}
            </button>
          )
        })}
      </div>

      <div className="flex-1 overflow-y-auto py-4">
        {isLoading && (
          <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
        )}

        {!isLoading && items.length === 0 && (
          <p className="text-sm text-muted-foreground">{t("shopping.empty")}</p>
        )}

        {items.length > 0 && (
          <ul className="space-y-2">
            {items.map((item) => {
              const checked = state.purchased.has(item.food_id)
              return (
                <li
                  key={item.food_id}
                  className="flex items-center gap-3 rounded-md px-1 py-1.5 hover:bg-muted/50"
                >
                  <input
                    type="checkbox"
                    id={`shop-${item.food_id}`}
                    checked={checked}
                    onChange={() => state.toggle(item.food_id)}
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
    </>
  )
}
