import { useEffect, useId } from "react"
import { useTranslation } from "react-i18next"

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
import type { DraftIngredient, Resolution } from "@/lib/api/import"
import type { LookupCandidate } from "@/lib/api/lookup"
import { leafInputFromCandidate } from "@/lib/api/foods"
import { normalizeUnit } from "@/lib/domain/units"
import { useEnsureLeaf, usePortions } from "@/lib/queries/foods"
import { useImportLineLookup } from "@/lib/queries/import"

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
}

interface RowProps {
  ing: DraftIngredient
  state: RowState
  lang: "de" | "en"
  onChange: (patch: Partial<RowState>) => void
}

export function IngredientResolveRow({ ing, state, lang, onChange }: RowProps) {
  const { t } = useTranslation()
  const ensureLeaf = useEnsureLeaf()
  const amountId = useId()
  const label = ing.name || ing.raw_text
  // Quantity-less lines ("Salz und Pfeffer") default to Skip; that is the
  // expected outcome, not a parse error to review.
  const optionalSkipped =
    ing.confidence === "unparsed" &&
    ing.amount === 0 &&
    state.resolution === "skip"

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

  const lookup = useImportLineLookup(ing.name, lang)
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

  // Auto-select only an exact catalogue match; prefix/contains matches
  // ("Zwiebelsuppe" for "Zwiebel") stay pickable but need a user choice.
  useEffect(() => {
    if (state.resolution !== "existing") return
    if (state.foodId) return
    if (!lookup.data) return
    const hit = results.find((r) => r.source === "local" && r.exact)
    if (hit && hit.existing_id) {
      onChange(relink(hit.existing_id, hit.name))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lookup.data])

  function link(leaf: { id: number; name: string }) {
    onChange({ resolution: "existing", ...relink(leaf.id, leaf.name) })
  }

  function onPickCandidate(r: LookupCandidate) {
    // A catalogue match is reused as-is; anything else becomes a new leaf
    // carrying the candidate's nutrition.
    if (r.existing_id) {
      link({ id: r.existing_id, name: r.name })
      return
    }
    ensureLeaf.mutate(leafInputFromCandidate(r), { onSuccess: link })
  }

  function onCreateManual() {
    // Macros stay null (not 0) so the completeness warning names the leaf.
    ensureLeaf.mutate(
      {
        kind: "leaf",
        name: ing.name,
        source: "manual",
        kcal_100g: null,
        protein_100g: null,
        fat_100g: null,
        carbs_100g: null,
      },
      { onSuccess: link }
    )
  }

  return (
    <div className="rounded border border-border p-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-40 flex-1">
          <div className="text-sm font-medium">
            {ing.amount > 0
              ? `${ing.amount} ${
                  ing.portion_grams !== null
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
          {optionalSkipped ? (
            <Badge
              variant="outline"
              className="mt-1 h-auto border-line whitespace-normal text-ink-muted"
            >
              {t("import.step2.optional_skipped")}
            </Badge>
          ) : (
            ing.confidence === "unparsed" && (
              <Badge
                variant="destructive"
                className="mt-1 h-auto whitespace-normal"
              >
                {t("import.step2.unparsed_warning")}
              </Badge>
            )
          )}
        </div>

        <div className="flex items-center gap-2">
          <Select
            value={state.resolution}
            onValueChange={(v) =>
              onChange({ resolution: v as Resolution, ...relink(null, null) })
            }
          >
            <SelectTrigger
              className="w-40"
              aria-label={t("import.step2.resolution_for", { name: label })}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="existing">
                {t("import.step2.res_existing")}
              </SelectItem>
              <SelectItem value="skip">{t("import.step2.res_skip")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      {state.resolution === "existing" && (
        <div className="mt-2 space-y-2">
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
          ) : lookup.isLoading ? (
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
                  className="flex w-full items-baseline justify-between gap-3 rounded border border-line px-2 py-1 text-left text-sm text-ink hover:bg-panel-sunken disabled:opacity-60"
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
              <Button
                variant="outline"
                size="sm"
                onClick={onCreateManual}
                disabled={ensureLeaf.isPending}
              >
                {t("import.step2.create_new")}
              </Button>
            </div>
          ) : (
            <div className="space-y-2">
              <p className="text-xs text-muted-foreground">
                {t("import.step2.no_match")}
              </p>
              <Button
                variant="outline"
                size="sm"
                onClick={onCreateManual}
                disabled={ensureLeaf.isPending}
              >
                {ensureLeaf.isPending
                  ? t("common.loading")
                  : t("import.step2.create_new")}
              </Button>
            </div>
          )}

          <div className="grid grid-cols-3 gap-2">
            <div>
              <Label htmlFor={amountId} className="text-xs">
                {t("import.step2.amount")}
              </Label>
              <Input
                id={amountId}
                aria-label={t("import.step2.amount_for", { name: label })}
                type="number"
                step="any"
                value={state.amount}
                onChange={(e) => onChange({ amount: Number(e.target.value) })}
              />
            </div>
            <div>
              <Label className="text-xs">{t("import.step2.unit")}</Label>
              <Select
                value={state.unit}
                onValueChange={(v) => onChange({ unit: v })}
              >
                <SelectTrigger
                  aria-label={t("import.step2.unit_for", { name: label })}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="g">g</SelectItem>
                  <SelectItem value="ml">ml</SelectItem>
                  {state.countUnit && (
                    <SelectItem value={state.countUnit}>
                      {t(`unit.${state.countUnit}.name`)}
                    </SelectItem>
                  )}
                </SelectContent>
              </Select>
            </div>
          </div>

          {isCountRow && (
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
        </div>
      )}
    </div>
  )
}
