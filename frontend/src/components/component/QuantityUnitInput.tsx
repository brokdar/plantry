import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import { Input } from "@/components/ui/input"
import { UnitSelect } from "@/components/ingredients/UnitSelect"
import {
  isCountUnit,
  normalizeUnit,
  resolveGrams,
  UNIT_DEFAULTS,
  type GramsSource,
  type PortionLookup,
} from "@/lib/domain/units"
import type { Food } from "@/lib/api/foods"
import { cn } from "@/lib/utils"

export interface QuantityUnitValue {
  amount: number
  unit: string
}

interface QuantityUnitInputProps {
  food: Food
  /** Parent-controlled value. The parent must seed an initial value via
   * `defaultQuantityValueForFood` (or the equivalent reducer init) before
   * mounting this component — there is no internal initial-emit. */
  value: QuantityUnitValue
  /** Fired whenever the user changes amount or unit. */
  onChange: (next: QuantityUnitValue) => void
  /**
   * Compact mode hides the quick-amount chips. Used for staged foods in the
   * food picker, where the chips would compete with the list.
   */
  compact?: boolean
  className?: string
}

/**
 * QuantityUnitInput is the *leaf* counterpart to `QuantityStepper`. It edits
 * the user-facing pair `(amount, unit)` for a leaf food on a plate; grams are
 * resolved server-side at save time so the cell's kcal stays consistent with
 * the food's portion table.
 *
 * Purely controlled — the parent owns the (amount, unit) state and seeds the
 * initial value (typically via `defaultQuantityValueForFood`). This keeps the
 * component free of effect-driven state writes.
 *
 * Why two components instead of one with a `mode` prop: leaf and composed
 * carry fundamentally different shapes (numeric+select+chips vs. integer
 * stepper) and the call sites already branch on `food.kind`. A single
 * component would only multiply the prop surface and obscure the kind
 * discrimination.
 */
export function QuantityUnitInput({
  food,
  value,
  onChange,
  compact = false,
  className,
}: QuantityUnitInputProps) {
  const { t } = useTranslation()

  const portions = useMemo<PortionLookup[]>(() => {
    if (food.kind !== "leaf" || !food.portions) return []
    return food.portions.map((p) => ({ unit: p.unit, grams: p.grams }))
  }, [food])

  // Local-only buffer for the temporarily-empty-string state of the amount
  // input. Tracked here (not pushed to the parent) so clearing the field
  // doesn't briefly emit `amount: 0` to consumers that round into kcal.
  const [emptyBuffer, setEmptyBuffer] = useState(false)

  function handleAmountChange(raw: string) {
    if (raw === "") {
      // Allow the field to be temporarily empty; we don't emit until the
      // user commits a positive number. Display "" via the controlled input.
      setEmptyBuffer(true)
      return
    }
    const n = Number(raw)
    if (!Number.isFinite(n) || n <= 0) return
    setEmptyBuffer(false)
    onChange({ ...value, amount: n })
  }

  function handleUnitChange(unit: string) {
    const canonical = normalizeUnit(unit) || unit
    // When the unit changes, snap the amount to a sensible default for the
    // new unit's vocabulary so the chip set below makes sense ("200" doesn't
    // belong on `1 apple`).
    const nextAmount = defaultAmountForUnit(canonical, portions, value.amount)
    setEmptyBuffer(false)
    onChange({ amount: nextAmount, unit: canonical })
  }

  const resolved = useMemo(
    () => resolveGrams(value.amount, value.unit, portions),
    [value.amount, value.unit, portions]
  )
  const sourceLabelKey = confidenceLabelKey(resolved.source)

  const chips = useMemo(
    () => quickChipsForUnit(value.unit, portions),
    [value.unit, portions]
  )

  const displayedAmount = emptyBuffer ? "" : value.amount

  return (
    <div
      className={cn("flex flex-col gap-2", compact && "gap-1.5", className)}
      data-testid="quantity-unit-input"
    >
      <div className="flex items-stretch gap-1.5">
        <Input
          type="number"
          inputMode="decimal"
          step="any"
          min={0}
          value={displayedAmount}
          onChange={(e) => handleAmountChange(e.target.value)}
          aria-label={t("plate.quantity.amount_label")}
          data-testid="quantity-unit-amount"
          className="h-11 w-20 shrink-0 border-line bg-panel text-body tabular-nums md:pointer-fine:h-9"
        />
        <div className="min-w-0 flex-1">
          <UnitSelect
            value={value.unit}
            onValueChange={handleUnitChange}
            portions={portions}
            placeholder={t("plate.quantity.unit_label")}
            testId="quantity-unit-unit"
            className="h-11 border-line bg-panel text-body md:pointer-fine:h-9"
          />
        </div>
        {sourceLabelKey && resolved.grams > 0 && (
          <span
            className="shrink-0 self-center text-caption text-ink-muted tabular-nums"
            data-testid="quantity-unit-grams"
            data-source={resolved.source}
          >
            ≈ {Math.round(resolved.grams)} g · {t(sourceLabelKey)}
          </span>
        )}
      </div>

      {!compact && chips.length > 0 && (
        <div
          className="-mx-0.5 flex flex-wrap gap-1"
          data-testid="quantity-unit-chips"
        >
          {chips.map((c) => {
            const active = !emptyBuffer && value.amount === c
            return (
              <button
                key={c}
                type="button"
                onClick={() => {
                  setEmptyBuffer(false)
                  onChange({ ...value, amount: c })
                }}
                aria-pressed={active}
                data-testid={`quantity-unit-chip-${c}`}
                className={cn(
                  "inline-flex min-h-11 min-w-11 items-center justify-center rounded-full border px-3 text-small tabular-nums transition-colors duration-150 ease-out focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-panel focus-visible:outline-none md:pointer-fine:min-h-8",
                  active
                    ? "border-accent bg-accent text-on-accent"
                    : "border-line text-ink hover:bg-panel-sunken"
                )}
              >
                {c}
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}

/**
 * defaultQuantityValueForFood seeds a fresh `(amount, unit)` for a leaf food
 * — the canonical initial value the parent should pass into `value`. Mirrors
 * the rule the legacy uncontrolled mode used to apply internally so existing
 * call sites don't have to re-derive it.
 */
export function defaultQuantityValueForFood(
  food: Food,
  recentUnit?: string | null
): QuantityUnitValue {
  const portions: PortionLookup[] =
    food.kind === "leaf" && food.portions
      ? food.portions.map((p) => ({ unit: p.unit, grams: p.grams }))
      : []
  const unit = pickDefaultUnit(food, recentUnit)
  return { unit, amount: defaultAmountForUnit(unit, portions) }
}

/**
 * pickDefaultUnit picks the initial unit per the spec:
 *  1. If `recentUnit` is set and resolvable, use it.
 *  2. Else, the first entry in the food's `portions` table.
 *  3. Else, `"g"` (the universal mass default).
 */
function pickDefaultUnit(food: Food, recentUnit?: string | null): string {
  if (recentUnit) {
    const canonical = normalizeUnit(recentUnit)
    if (canonical) return canonical
  }
  if (food.kind === "leaf" && food.portions && food.portions.length > 0) {
    const first = normalizeUnit(food.portions[0]!.unit)
    if (first) return first
  }
  return "g"
}

/**
 * defaultAmountForUnit picks a fresh amount for a unit. Count-like (1) for
 * portion units; 100 g for grams; 100 ml for volumes; carries the previous
 * amount otherwise. `prev` is honoured when the unit kind is the same so
 * switching `200 g → 200 oz` doesn't surprise the user with a snap to 100.
 */
function defaultAmountForUnit(
  unit: string,
  portions: PortionLookup[],
  prev?: number
): number {
  const canonical = normalizeUnit(unit) || unit
  if (portions.some((p) => normalizeUnit(p.unit) === canonical)) {
    return prev ?? 1
  }
  const def = UNIT_DEFAULTS[canonical]
  if (def) {
    if (def.kind === "mass") return prev && prev >= 10 ? prev : 100
    if (def.kind === "volume") return prev && prev >= 10 ? prev : 100
  }
  if (isCountUnit(canonical)) return prev ?? 1
  return prev ?? 1
}

/**
 * quickChipsForUnit returns the suggested quick-amount chips for the
 * currently-selected unit. Mirrors the spec in plan §3.1.
 */
function quickChipsForUnit(unit: string, portions: PortionLookup[]): number[] {
  const canonical = normalizeUnit(unit) || unit
  if (portions.some((p) => normalizeUnit(p.unit) === canonical)) {
    return [1, 2, 3]
  }
  if (isCountUnit(canonical)) return [1, 2, 3]
  const def = UNIT_DEFAULTS[canonical]
  if (def?.kind === "mass") {
    if (canonical === "g") return [50, 100, 150, 200, 300]
    return [1, 2, 5]
  }
  if (def?.kind === "volume") {
    return [100, 200, 250, 500]
  }
  return []
}

/**
 * confidenceLabelKey mirrors the recipe editor's `GramsSourceBadge` mapping:
 *  - direct (raw grams), portion (food's own table), and default (mass-unit
 *    conversions like kg/oz/lb) are all *exact* — typed grams can't be more
 *    exact than themselves, and the conversion factors are mathematical, not
 *    estimates.
 *  - fallback (volume on a non-volume food) and manual (user override on a
 *    count unit without a portion entry) are estimates.
 */
function confidenceLabelKey(source: GramsSource): string | null {
  switch (source) {
    case "direct":
    case "portion":
    case "default":
      return "plate.quantity.confidence.portion"
    case "fallback":
    case "manual":
      return "plate.quantity.confidence.fallback"
    case "unresolved":
      return null
    default:
      return null
  }
}
