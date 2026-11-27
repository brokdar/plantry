import { useEffect, useId, useState } from "react"
import { useTranslation } from "react-i18next"

import { DietIcon } from "@/components/recipes/DietIcon"
import { useIsPhone } from "@/components/plan/use-is-phone"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetTitle,
} from "@/components/ui/sheet"
import type { DraftIngredient, Resolution } from "@/lib/api/import"
import type { LookupCandidate } from "@/lib/api/lookup"
import { leafInputFromCandidate } from "@/lib/api/foods"
import { normalizeUnit } from "@/lib/domain/units"
import { useEnsureLeaf, usePortions } from "@/lib/queries/foods"
import { useImportLineLookup } from "@/lib/queries/import"

import { rowProblem } from "./draft-rows"

export type RowState = {
  resolution: Resolution
  foodId: number | null
  foodName: string | null
  amount: number
  unit: string
  /** The draft's count unit ("piece", "clove"), or null for mass/volume. */
  countUnit: string | null
  /** Grams per count unit: the edited value, else the linked leaf's
   *  portion, else the draft's estimate. */
  portionGrams: number | null
  /** True once the user typed a weight; only then is it sent to the
   *  backend, which writes it to the leaf's portion. */
  portionEdited: boolean
  /** A to-taste seasoning line (MATCH-6): saved without an amount. */
  toTaste: boolean
  /** The amount is the import's proposal until the user changes it. */
  proposed: boolean
  /** Removed from the recipe (MATCH-5's fourth fix). */
  removed: boolean
  /** The name the row searches the catalogue, FDC and OFF for. */
  query: string
}

interface RowProps {
  ing: DraftIngredient
  state: RowState
  lang: "de" | "en"
  onChange: (patch: Partial<RowState>) => void
}

/** One ingredient line of the review: on desktop the line and its editor
 *  side by side, on phones a summary that opens the editor in a sheet. */
export function IngredientResolveRow({ ing, state, lang, onChange }: RowProps) {
  const { t } = useTranslation()
  const phone = useIsPhone()
  const [open, setOpen] = useState(false)
  const label = ing.name || ing.raw_text
  const problem = rowProblem(state)

  if (state.removed) {
    return (
      <div className="flex items-center justify-between gap-2 rounded border border-dashed border-line px-3 py-2 text-sm text-ink-muted">
        <span className="line-through">{ing.raw_text}</span>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onChange({ removed: false })}
        >
          {t("import.step2.undo_remove")}
        </Button>
      </div>
    )
  }

  const editor = (
    <RowEditor ing={ing} state={state} lang={lang} onChange={onChange} />
  )

  if (phone) {
    return (
      <>
        <button
          type="button"
          className="flex min-h-11 w-full items-center justify-between gap-3 rounded border border-border px-3 py-2 text-left text-sm"
          aria-label={t("import.step2.edit_line", { name: label })}
          onClick={() => setOpen(true)}
        >
          <span className="min-w-0 flex-1">
            <span className="block truncate font-medium">
              {state.foodName ?? label}
            </span>
            <span className="block truncate text-xs text-ink-muted">
              {ing.raw_text}
            </span>
          </span>
          <RowStatus state={state} />
        </button>
        <Sheet open={open} onOpenChange={setOpen}>
          <SheetContent
            side="bottom"
            className="max-h-[90dvh] gap-3 overflow-y-auto rounded-t-sheet bg-panel p-4 text-ink"
          >
            <SheetTitle>{label}</SheetTitle>
            <SheetDescription>{ing.raw_text}</SheetDescription>
            {editor}
            <Button onClick={() => setOpen(false)} className="h-11">
              {t("import.step2.done")}
            </Button>
          </SheetContent>
        </Sheet>
      </>
    )
  }

  return (
    <div
      className="grid gap-3 rounded border border-border p-3 md:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]"
      data-problem={problem ?? undefined}
    >
      <LineInfo ing={ing} state={state} />
      {editor}
    </div>
  )
}

function RowStatus({ state }: { state: RowState }) {
  const { t } = useTranslation()
  const problem = rowProblem(state)
  if (problem) {
    return (
      <Badge variant="destructive" className="shrink-0">
        {t(`import.step2.problem_${problem}`)}
      </Badge>
    )
  }
  return (
    <span className="shrink-0 text-xs text-ink-muted tabular-nums">
      {state.toTaste
        ? t("ingredient.to_taste")
        : `${state.amount} ${state.unit}`}
    </span>
  )
}

/** Tried sources in one line: "Library · USDA “onion” · Open Food Facts". */
function triedLine(tried: string[], t: (k: string) => string): string {
  return tried
    .map((s) => {
      if (s === "catalogue") return t("import.step2.source_local")
      const [src, ...rest] = s.split(":")
      const q = rest[rest.length - 1]
      return `${t(`import.step2.source_${src}`)} “${q}”`
    })
    .join(" · ")
}

function LineInfo({ ing, state }: { ing: DraftIngredient; state: RowState }) {
  const { t } = useTranslation()
  const m = ing.match
  return (
    <div className="min-w-0">
      <div className="text-sm font-medium">
        {ing.amount > 0 && !ing.proposed
          ? `${ing.amount} ${
              ing.portion_grams !== null || ing.needs_weight
                ? t(`unit.${ing.unit}.name`, { count: ing.amount })
                : ing.unit || ing.original_unit
            } `
          : ""}
        {ing.name || ing.raw_text}
        {ing.note && (
          <span className="text-muted-foreground"> — {ing.note}</span>
        )}
      </div>
      <div className="text-xs text-muted-foreground">{ing.raw_text}</div>
      <DietIcon
        dietClass={ing.diet_class}
        withLabel
        className="mt-1 text-caption text-ink-muted"
      />
      {ing.warning && (
        <Badge
          variant="outline"
          className="mt-1 h-auto border-amber-400 whitespace-normal text-amber-900"
        >
          {t(`import.step2.warning_${ing.warning}`)}
        </Badge>
      )}
      {!state.foodId && m && m.status === "unmatched" && (
        <p className="mt-1 text-xs text-ink-muted">
          {m.reason && t(`import.step2.reason_${m.reason}`)}
          {m.tried.length > 0 &&
            ` ${t("import.step2.tried", { list: triedLine(m.tried, t) })}`}
        </p>
      )}
    </div>
  )
}

function RowEditor({ ing, state, lang, onChange }: RowProps) {
  const { t } = useTranslation()
  const ensureLeaf = useEnsureLeaf()
  const amountId = useId()
  const label = ing.name || ing.raw_text
  const [search, setSearch] = useState(state.query)
  const [creating, setCreating] = useState(false)

  // A weight typed for one leaf must not be written to another: changing the
  // linked food drops the edit and falls back to the estimate until the new
  // leaf's saved portion loads.
  function relink(foodId: number | null, foodName: string | null) {
    return {
      foodId,
      foodName,
      portionGrams: ing.portion_grams,
      portionEdited: false,
    } satisfies Partial<RowState>
  }

  const lookup = useImportLineLookup(state.query, lang)
  const results = lookup.data?.results ?? []

  const isCountRow = state.countUnit !== null && state.unit === state.countUnit
  // The linked leaf's own weight for the count unit beats the estimate.
  const portions = usePortions(isCountRow && state.foodId ? state.foodId : 0)
  const leafPortion = portions.data?.items.find(
    (p) => normalizeUnit(p.unit) === state.countUnit
  )
  useEffect(() => {
    if (!isCountRow || state.portionEdited || !portions.data) return
    const grams = leafPortion?.grams ?? ing.portion_grams
    if (grams !== state.portionGrams) onChange({ portionGrams: grams })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [portions.data, leafPortion?.grams])

  function link(leaf: { id: number; name: string }, resolution: Resolution) {
    onChange({ resolution, ...relink(leaf.id, leaf.name) })
  }

  function onPickCandidate(r: LookupCandidate) {
    // A catalogue match is reused as-is; anything else becomes a new leaf
    // carrying the candidate's nutrition.
    if (r.existing_id) {
      link({ id: r.existing_id, name: r.name }, "existing")
      return
    }
    ensureLeaf.mutate(leafInputFromCandidate(r), {
      onSuccess: (leaf) => link(leaf, "new"),
    })
  }

  return (
    <div className="space-y-2">
      {state.foodId ? (
        <div className="flex items-center gap-2 text-sm">
          <Badge>{t("import.step2.resolved_label")}</Badge>
          <span>{state.foodName}</span>
          <button
            className="text-xs text-muted-foreground underline underline-offset-2"
            onClick={() => onChange(relink(null, null))}
          >
            {t("import.step2.change")}
          </button>
        </div>
      ) : (
        <div className="space-y-2">
          <form
            className="flex gap-2"
            onSubmit={(e) => {
              e.preventDefault()
              onChange({ query: search.trim() })
            }}
          >
            <Input
              aria-label={t("import.step2.search_for", { name: label })}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="h-9"
            />
            <Button type="submit" variant="outline" size="sm">
              {t("import.step2.search")}
            </Button>
          </form>
          {lookup.isLoading ? (
            <p className="text-xs text-muted-foreground">
              {t("import.step2.searching")}
            </p>
          ) : results.length > 0 ? (
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">
                {t("import.step2.pick_match")}
              </p>
              {results.map((r, i) => (
                <button
                  key={`${r.source}-${r.existing_id ?? r.fdc_id ?? r.barcode ?? r.name}-${i}`}
                  type="button"
                  className="flex min-h-9 w-full items-baseline justify-between gap-3 rounded border border-line px-2 py-1 text-left text-sm text-ink hover:bg-panel-sunken disabled:opacity-60"
                  onClick={() => onPickCandidate(r)}
                  disabled={ensureLeaf.isPending}
                >
                  <span>{r.source_name || r.name}</span>
                  <span className="flex shrink-0 gap-3 text-xs text-ink-muted tabular-nums">
                    {r.kcal_100g != null && (
                      <span>
                        {t("import.step2.kcal_100g", {
                          kcal: Math.round(r.kcal_100g),
                        })}
                      </span>
                    )}
                    <span>{t(`import.step2.source_${r.source}`)}</span>
                  </span>
                </button>
              ))}
            </div>
          ) : (
            <p className="text-xs text-muted-foreground">
              {lookup.data?.unavailable_reason
                ? t(`import.step2.lookup_${lookup.data.unavailable_reason}`)
                : t("import.step2.no_match")}
            </p>
          )}
          {creating ? (
            <NewLeafForm
              name={state.query}
              pending={ensureLeaf.isPending}
              onCancel={() => setCreating(false)}
              onCreate={(macros) =>
                ensureLeaf.mutate(
                  {
                    kind: "leaf",
                    name: state.query,
                    source: "manual",
                    ...macros,
                  },
                  { onSuccess: (leaf) => link(leaf, "new") }
                )
              }
            />
          ) : (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setCreating(true)}
            >
              {t("import.step2.create_with_nutrition")}
            </Button>
          )}
        </div>
      )}

      {state.toTaste ? (
        <div className="flex items-center gap-2 text-sm">
          <Badge variant="secondary">{t("ingredient.to_taste")}</Badge>
          <button
            type="button"
            className="text-xs text-muted-foreground underline underline-offset-2"
            onClick={() => onChange({ toTaste: false })}
          >
            {t("import.step2.enter_amount")}
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-3 items-end gap-2">
          <div>
            <Label htmlFor={amountId} className="text-xs">
              {t("import.step2.amount")}
            </Label>
            <Input
              id={amountId}
              aria-label={t("import.step2.amount_for", { name: label })}
              type="number"
              step="any"
              value={state.amount || ""}
              aria-invalid={!(state.amount > 0) || undefined}
              onChange={(e) =>
                onChange({ amount: Number(e.target.value), proposed: false })
              }
            />
          </div>
          <div>
            <Label className="text-xs">{t("import.step2.unit")}</Label>
            <Select
              value={state.unit}
              onValueChange={(v) => onChange({ unit: v, proposed: false })}
            >
              <SelectTrigger
                aria-label={t("import.step2.unit_for", { name: label })}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="g">{t("units.g")}</SelectItem>
                <SelectItem value="ml">ml</SelectItem>
                <SelectItem value="tbsp">{t("unit.tbsp.name")}</SelectItem>
                <SelectItem value="tsp">{t("unit.tsp.name")}</SelectItem>
                {state.countUnit &&
                  !["g", "ml", "tbsp", "tsp"].includes(state.countUnit) && (
                    <SelectItem value={state.countUnit}>
                      {t(`unit.${state.countUnit}.name`)}
                    </SelectItem>
                  )}
              </SelectContent>
            </Select>
          </div>
          {state.proposed && (
            <Badge variant="outline" className="mb-2 w-fit">
              {t("import.step2.proposed")}
            </Badge>
          )}
        </div>
      )}

      {isCountRow && !state.toTaste && (
        <div className="flex flex-wrap items-center gap-2 text-small text-ink-muted">
          <span aria-hidden="true">
            {t("import.step2.portion_estimate", {
              unit: t(`unit.${state.countUnit}.name`, { count: 1 }),
            })}
          </span>
          <Input
            aria-label={t("import.step2.portion_grams_for", {
              unit: t(`unit.${state.countUnit}.name`, { count: 1 }),
              name: label,
            })}
            type="number"
            inputMode="decimal"
            min={0}
            step="any"
            className="h-11 w-24 tabular-nums"
            value={state.portionGrams ?? ""}
            aria-invalid={!((state.portionGrams ?? 0) > 0) || undefined}
            onChange={(e) =>
              onChange({
                portionGrams:
                  e.target.value === "" ? null : Number(e.target.value),
                portionEdited: true,
              })
            }
          />
          <span>{t("units.g")}</span>
          {!state.portionEdited && (
            <span>
              {leafPortion
                ? t("import.step2.portion_saved")
                : t("import.step2.portion_estimated")}
            </span>
          )}
        </div>
      )}

      <button
        type="button"
        className="text-xs text-danger underline underline-offset-2"
        onClick={() => onChange({ removed: true })}
      >
        {t("import.step2.remove_line", { name: label })}
      </button>
    </div>
  )
}

const MACROS = ["kcal_100g", "protein_100g", "fat_100g", "carbs_100g"] as const
type Macros = Record<(typeof MACROS)[number], number>

/** A new leaf needs nutrition per 100 g (P3): all four core values. */
export function NewLeafForm({
  name,
  pending,
  onCreate,
  onCancel,
  hint,
  submitLabel,
}: {
  name: string
  pending: boolean
  onCreate: (m: Macros) => void
  onCancel: () => void
  /** Replaces the new-ingredient hint (legacy fix of an existing leaf). */
  hint?: string
  submitLabel?: string
}) {
  const { t } = useTranslation()
  const [values, setValues] = useState<Record<string, string>>({})
  const parsed = MACROS.map((k) => Number(values[k]))
  const valid = MACROS.every(
    (k, i) => values[k] !== undefined && values[k] !== "" && parsed[i] >= 0
  )
  return (
    <form
      className="space-y-2 rounded border border-line p-2"
      onSubmit={(e) => {
        e.preventDefault()
        if (!valid) return
        onCreate(
          Object.fromEntries(MACROS.map((k, i) => [k, parsed[i]])) as Macros
        )
      }}
    >
      <p className="text-xs text-ink-muted">
        {hint ?? t("import.step2.new_leaf_hint", { name })}
      </p>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        {MACROS.map((k) => (
          <div key={k}>
            <Label className="text-xs">{t(`import.step2.new_${k}`)}</Label>
            <Input
              aria-label={t(`import.step2.new_${k}`)}
              type="number"
              inputMode="decimal"
              min={0}
              step="any"
              value={values[k] ?? ""}
              onChange={(e) => setValues({ ...values, [k]: e.target.value })}
            />
          </div>
        ))}
      </div>
      <div className="flex gap-2">
        <Button type="submit" size="sm" disabled={!valid || pending}>
          {submitLabel ?? t("import.step2.create_leaf")}
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
      </div>
    </form>
  )
}
