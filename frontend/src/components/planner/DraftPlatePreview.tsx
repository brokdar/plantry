import { ChevronDown, Plus } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { MacroDistributionBar } from "@/components/editorial/macros"
import type { MacrosResponse } from "@/lib/api/plates"
import type { Food } from "@/lib/api/foods"
import type { PlateComponent } from "@/lib/api/plates"
import { resolveGrams } from "@/lib/domain/units"
import { cn } from "@/lib/utils"

import { SlotHero, type SlotHeroComponent } from "./SlotHero"
import type { TrayItem, TrayQuantity } from "./ComponentTraySheet"

/** ExistingComponent is the projection of a `PlateComponent` the preview needs
 *  to render the "what was already on the plate" muted layer. The tray sheet
 *  resolves food_id → Food once at the call site so this component stays
 *  presentational. */
export interface DraftPlatePreviewExistingItem {
  pc: PlateComponent
  food: Food | undefined
}

interface DraftPlatePreviewProps {
  items: TrayItem[]
  /** Per-portion / per-100g macros the parent already has cached (food_id →
   *  kcal + macros). The preview multiplies by the staged quantity to render
   *  the running total + macro bar. */
  macrosByFood: Map<number, MacrosResponse>
  /** Components already on the plate (greyed, prepended). When present, the
   *  preview makes the existing-vs-staged distinction visible so the user
   *  knows what they're editing. */
  existing?: DraftPlatePreviewExistingItem[]
  /** Per-slot kcal target (optional) — passed through to support a future
   *  ring/tone treatment. Unused in the bar today but accepted to keep the
   *  prop shape stable. */
  dayKcalTarget?: number | null
  /** When `true`, render the bottom-sheet collapsed shell (single-line
   *  summary, tap to expand). Desktop right rail keeps the full preview
   *  always-on. */
  collapsible?: boolean
  /** Monotonic counter that ticks every time the parent restages an
   *  already-staged food. Paired with `bumpedFoodId` it lets the matching
   *  staged pill re-key and replay its enter animation, so the user sees
   *  visible feedback that the re-tap bumped the quantity instead of
   *  duplicating the row. */
  bumpToken?: { foodId: number; nonce: number } | null
}

/**
 * DraftPlatePreview is the visual anchor of the picker tray: it shows what
 * the plate will look like after the staged additions land, with the
 * existing components muted underneath so the user can see what they're
 * editing.
 *
 * Strictly presentational — no queries, no state beyond the local
 * collapsed/expanded toggle for the mobile bottom-sheet variant.
 */
export function DraftPlatePreview({
  items,
  macrosByFood,
  existing,
  collapsible = false,
  bumpToken,
}: DraftPlatePreviewProps) {
  const { t } = useTranslation()

  // Mobile: collapsed by default to give the search results vertical room.
  // The single-line summary still surfaces the kcal so the user never loses
  // the running total just because they haven't tapped to expand.
  const [expanded, setExpanded] = useState(false)

  const existingItems = existing ?? []
  const hasStaged = items.length > 0
  const hasExisting = existingItems.length > 0
  const isEmpty = !hasStaged && !hasExisting

  const total = computeRunningTotal(items, macrosByFood)

  if (collapsible && !expanded) {
    return (
      <CollapsedSummary
        kcal={total.kcal}
        stagedCount={items.length}
        existingCount={existingItems.length}
        isEmpty={isEmpty}
        onExpand={() => setExpanded(true)}
      />
    )
  }

  return (
    <section
      data-testid="draft-plate-preview"
      aria-label={t("tray.preview.aria_label")}
      className={cn(
        "flex flex-col gap-3 border-b border-outline-variant/40 bg-surface-container-low/40 px-5 py-4"
      )}
    >
      <header className="flex items-center justify-between gap-2">
        <span className="font-heading text-[10.5px] font-bold tracking-[0.22em] text-on-surface-variant uppercase">
          {t("tray.preview.title")}
        </span>
        {collapsible && (
          <button
            type="button"
            onClick={() => setExpanded(false)}
            data-testid="draft-plate-preview-collapse"
            className="font-heading text-[10px] font-bold tracking-[0.16em] text-on-surface-variant uppercase hover:text-on-surface"
          >
            {t("common.collapse")}
          </button>
        )}
      </header>

      {isEmpty ? (
        <EmptyState />
      ) : (
        <>
          <PreviewHero existing={existingItems} staged={items} />
          <PillsRow
            existing={existingItems}
            staged={items}
            bumpToken={bumpToken ?? null}
          />
          <RunningTotal total={total} hasStaged={hasStaged} />
        </>
      )}
    </section>
  )
}

function CollapsedSummary({
  kcal,
  stagedCount,
  existingCount,
  isEmpty,
  onExpand,
}: {
  kcal: number
  stagedCount: number
  existingCount: number
  isEmpty: boolean
  onExpand: () => void
}) {
  const { t } = useTranslation()
  // The summary describes whichever story the tray is currently telling: when
  // the user has staged additions, show "Adding N · +X kcal"; otherwise fall
  // back to the existing-on-plate count so the count and kcal always agree on
  // what they're counting.
  const hasStaged = stagedCount > 0
  const summary = isEmpty
    ? t("tray.preview.empty")
    : hasStaged
      ? t("tray.preview.summary", {
          count: stagedCount,
          kcal: Math.round(kcal),
        })
      : t("tray.preview.summary_existing", { count: existingCount })
  return (
    <button
      type="button"
      onClick={onExpand}
      data-testid="draft-plate-preview-collapsed"
      className="flex w-full items-center justify-between gap-3 border-b border-outline-variant/40 bg-surface-container-low/40 px-5 py-3 text-left transition-colors hover:bg-surface-container-low"
    >
      <span className="flex flex-col gap-0.5">
        <span className="font-heading text-[10px] font-bold tracking-[0.22em] text-on-surface-variant uppercase">
          {t("tray.preview.title")}
        </span>
        <span className="text-[12.5px] font-medium text-on-surface">
          {summary}
        </span>
      </span>
      <span className="flex items-center gap-1.5 text-on-surface-variant">
        {hasStaged && (
          <span
            className="font-mono text-[12px] font-semibold text-on-surface tabular-nums"
            data-testid="tray-running-kcal"
          >
            +{Math.round(kcal)} {t("macro.kcal")}
          </span>
        )}
        <ChevronDown className="h-3.5 w-3.5 -rotate-90" aria-hidden />
      </span>
    </button>
  )
}

function EmptyState() {
  const { t } = useTranslation()
  return (
    <div
      data-testid="draft-plate-preview-empty"
      className="flex flex-col items-center gap-1 rounded-xl border border-dashed border-outline-variant/50 bg-surface-container-lowest/60 px-4 py-5 text-center"
    >
      <span className="font-heading text-[11px] font-bold tracking-[0.22em] text-on-surface-variant/80 uppercase">
        {t("tray.preview.empty")}
      </span>
      <span className="text-[12px] text-on-surface-variant/70">
        {t("tray.preview.empty_hint")}
      </span>
    </div>
  )
}

function PreviewHero({
  existing,
  staged,
}: {
  existing: DraftPlatePreviewExistingItem[]
  staged: TrayItem[]
}) {
  // Build the SlotHero component list from existing components first, then
  // staged. This keeps the visual hierarchy consistent with the order in
  // which the plate is being assembled and makes the existing items
  // dominate the hero only when there are no staged additions yet.
  const heroComponents: SlotHeroComponent[] = []
  for (const ex of existing) {
    if (!ex.food) continue
    heroComponents.push({
      imagePath: ex.food.image_path ?? null,
      role: ex.food.kind === "composed" ? (ex.food.role ?? null) : null,
      name: ex.food.name,
    })
  }
  for (const it of staged) {
    heroComponents.push({
      imagePath: it.food.image_path ?? null,
      role: it.food.kind === "composed" ? (it.food.role ?? null) : null,
      name: it.food.name,
    })
  }
  return (
    <div
      className="overflow-hidden rounded-xl border border-outline-variant/40"
      data-testid="draft-plate-preview-hero"
    >
      <SlotHero
        components={heroComponents}
        heroRoleLabel={null}
        variant="compact"
      />
    </div>
  )
}

function PillsRow({
  existing,
  staged,
  bumpToken,
}: {
  existing: DraftPlatePreviewExistingItem[]
  staged: TrayItem[]
  bumpToken: { foodId: number; nonce: number } | null
}) {
  const { t } = useTranslation()
  return (
    <ul
      role="list"
      data-testid="draft-plate-preview-pills"
      className="flex flex-wrap gap-1.5"
    >
      {existing.map((ex, idx) => {
        const label = ex.food?.name ?? t("tray.preview.unknown_food")
        const qty = formatExistingQuantity(ex.pc)
        return (
          <li
            key={`existing-${ex.pc.id}-${idx}`}
            data-testid={`draft-plate-preview-existing-${ex.pc.id}`}
            data-state="existing"
            className="inline-flex items-center gap-1.5 rounded-full border border-outline-variant/40 bg-surface-container-lowest/60 px-2.5 py-1 text-[11.5px] text-on-surface-variant/80"
          >
            <span className="max-w-[160px] truncate">{label}</span>
            {qty && (
              <span className="font-mono text-[10.5px] text-on-surface-variant/60 tabular-nums">
                {qty}
              </span>
            )}
          </li>
        )
      })}
      {staged.map((it) => {
        const qty = formatTrayQuantity(it.quantity)
        // When the parent reports a bump for this food, mix the bump nonce
        // into the key so React re-mounts the pill — that replays the
        // enter animation and visibly confirms the re-tap landed.
        const bumpNonce =
          bumpToken && bumpToken.foodId === it.food.id ? bumpToken.nonce : 0
        return (
          <li
            key={`staged-${it.food.id}-${bumpNonce}`}
            data-testid={`draft-plate-preview-staged-${it.food.id}`}
            data-state="staged"
            className="inline-flex items-center gap-1.5 rounded-full border border-primary/55 bg-primary/12 px-2.5 py-1 text-[11.5px] font-medium text-on-surface motion-safe:animate-in motion-safe:duration-200 motion-safe:fade-in motion-safe:slide-in-from-bottom-1"
          >
            <Plus className="h-3 w-3 text-primary" aria-hidden />
            <span className="max-w-[160px] truncate">{it.food.name}</span>
            {qty && (
              <span className="font-mono text-[10.5px] text-primary tabular-nums">
                {qty}
              </span>
            )}
          </li>
        )
      })}
    </ul>
  )
}

interface RunningTotalValue {
  kcal: number
  protein: number
  carbs: number
  fat: number
}

function RunningTotal({
  total,
  hasStaged,
}: {
  total: RunningTotalValue
  hasStaged: boolean
}) {
  const { t } = useTranslation()
  // The kcal here is the *addition* — staged items only — so we show "+X kcal"
  // and label the row "Adding". Existing components keep their kcal in the
  // slot sheet header where it belongs to the whole-plate total.
  const kcalRounded = Math.round(total.kcal)
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline justify-between gap-2">
        <span className="font-heading text-[10px] font-bold tracking-[0.22em] text-on-surface-variant uppercase">
          {t("tray.running_total")}
        </span>
        {/* Re-key on the rounded kcal so the value briefly fades in on each
         *  change — the kcal is the running-total's payload, so a small
         *  motion cue keeps the user oriented while typing quantities. */}
        <span
          key={kcalRounded}
          className="font-mono text-[13px] font-semibold text-on-surface tabular-nums motion-safe:animate-in motion-safe:duration-200 motion-safe:fade-in"
          data-testid="tray-running-kcal"
        >
          {hasStaged ? "+" : ""}
          {kcalRounded} {t("macro.kcal")}
        </span>
      </div>
      {hasStaged && (
        <div className="motion-safe:animate-in motion-safe:duration-300 motion-safe:fade-in motion-safe:slide-in-from-bottom-1">
          <MacroDistributionBar
            thickness="sm"
            track="surface-container"
            mode="kcal"
            values={{
              protein: total.protein,
              carbs: total.carbs,
              fat: total.fat,
            }}
            label={t("tray.preview.macro_bar_label", {
              kcal: kcalRounded,
            })}
          />
        </div>
      )}
    </div>
  )
}

// ── Helpers ──────────────────────────────────────────────────────────

/** computeRunningTotal aggregates kcal + macros across the staged tray
 *  items. We mirror the multiplier rule the backend uses (composed →
 *  portions; leaf → grams ÷ 100), but resolve grams in-memory using the
 *  food's portion table so the preview can update without an extra fetch.
 *  Existing plate components are *not* included here: their macros already
 *  show up in the cell / slot sheet, and the running total speaks to what
 *  the user is *adding*. */
function computeRunningTotal(
  items: TrayItem[],
  macrosByFood: Map<number, MacrosResponse>
): RunningTotalValue {
  let kcal = 0
  let protein = 0
  let carbs = 0
  let fat = 0
  for (const it of items) {
    const m = macrosByFood.get(it.food.id)
    if (!m) continue
    const mult = trayItemMultiplier(it)
    kcal += m.kcal * mult
    protein += m.protein * mult
    carbs += m.carbs * mult
    fat += m.fat * mult
  }
  return { kcal, protein, carbs, fat }
}

function trayItemMultiplier(item: TrayItem): number {
  if (item.quantity.kind === "composed") return item.quantity.portions
  if (item.food.kind !== "leaf") return 0
  // Mirror the canonical resolver used by the food editor + ComponentTraySheet
  // so the preview, footer, and server agree on what one staged amount means.
  const portions = item.food.portions ?? []
  const r = resolveGrams(item.quantity.amount, item.quantity.unit, portions)
  return r.grams / 100
}

function formatTrayQuantity(q: TrayQuantity): string {
  if (q.kind === "composed") {
    return `× ${q.portions}`
  }
  return `${q.amount} ${q.unit}`
}

function formatExistingQuantity(pc: PlateComponent): string | null {
  if (pc.portions != null) return `× ${pc.portions}`
  if (pc.amount != null && pc.unit != null) return `${pc.amount} ${pc.unit}`
  return null
}
