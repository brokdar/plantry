import { useEffect } from "react"
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
import { useEnsureLeaf } from "@/lib/queries/foods"
import { useImportLineLookup } from "@/lib/queries/import"

export type RowState = {
  resolution: Resolution
  foodId: number | null
  foodName: string | null
  amount: number
  unit: string
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

  const lookup = useImportLineLookup(ing.name, lang)
  const results = lookup.data?.results ?? []

  // Auto-select the first returned candidate that has an existing_id.
  useEffect(() => {
    if (state.resolution !== "existing") return
    if (state.foodId) return
    if (!lookup.data) return
    const hit = results.find((r) => r.existing_id)
    if (hit && hit.existing_id) {
      onChange({ foodId: hit.existing_id, foodName: hit.name })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lookup.data])

  function link(leaf: { id: number; name: string }) {
    onChange({ resolution: "existing", foodId: leaf.id, foodName: leaf.name })
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
    ensureLeaf.mutate(
      { kind: "leaf", name: ing.name, source: "manual" },
      { onSuccess: link }
    )
  }

  return (
    <div className="rounded border border-border p-3">
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1">
          <div className="text-sm font-medium">
            {ing.amount > 0
              ? `${ing.amount} ${ing.unit || ing.original_unit} `
              : ""}
            {ing.name || ing.raw_text}
            {ing.note && (
              <span className="text-muted-foreground"> — {ing.note}</span>
            )}
          </div>
          <div className="text-xs text-muted-foreground">{ing.raw_text}</div>
          {ing.confidence === "unparsed" && (
            <Badge variant="destructive" className="mt-1">
              {t("import.step2.unparsed_warning")}
            </Badge>
          )}
        </div>

        <div className="flex items-center gap-2">
          <Select
            value={state.resolution}
            onValueChange={(v) =>
              onChange({ resolution: v as Resolution, foodId: null })
            }
          >
            <SelectTrigger className="w-40">
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
                onClick={() => onChange({ foodId: null, foodName: null })}
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
              <Label className="text-xs">{t("import.step2.amount")}</Label>
              <Input
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
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="g">g</SelectItem>
                  <SelectItem value="ml">ml</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
