import { memo, useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { useWatch, type UseFormReturn } from "react-hook-form"
import { Lightbulb, Trash2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { UnitSelect } from "@/components/ingredients/UnitSelect"
import { cn } from "@/lib/utils"
import { useFood, usePortions, useUpsertPortion } from "@/lib/queries/foods"
import { isCountUnit, normalizeUnit, resolveGrams } from "@/lib/domain/units"
import type { LeafFood } from "@/lib/api/foods"
import type { ComposedFoodFormValues } from "@/lib/schemas/food"

import { GramsSourceBadge } from "./GramsSourceBadge"
import { IngredientCombobox } from "./IngredientCombobox"

type IngredientRowProps = {
  index: number
  form: UseFormReturn<ComposedFoodFormValues>
  onRemove: (index: number) => void
}

function IngredientRowImpl({ index, form, onRemove }: IngredientRowProps) {
  const { t } = useTranslation()
  const { control } = form

  const foodId = useWatch({
    control,
    name: `children.${index}.child_id`,
  })
  const ingredientName = useWatch({
    control,
    name: `children.${index}.child_name`,
  })
  const unit = useWatch({ control, name: `children.${index}.unit` })
  const rawAmount = useWatch({ control, name: `children.${index}.amount` })
  const rawGrams = useWatch({ control, name: `children.${index}.grams` })
  const amount = Number(rawAmount) || 0
  const currentGrams = Number(rawGrams) || 0

  const { data: portionsData, isSuccess: portionsLoaded } = usePortions(foodId)
  const portions = portionsData?.items ?? []
  const { data: ingredientDetail } = useFood(foodId)
  const upsertPortion = useUpsertPortion()

  const [addPortionOpen, setAddPortionOpen] = useState(false)
  const [addPortionGrams, setAddPortionGrams] = useState("")
  const [proactiveHintDismissed, setProactiveHintDismissed] = useState(false)

  const resolved = resolveGrams(amount, unit, portions, currentGrams)

  useEffect(() => {
    if (!foodId) return
    if (resolved.source === "manual" || resolved.source === "unresolved") return
    if (Math.abs(resolved.grams - currentGrams) < 0.001) return
    form.setValue(`children.${index}.grams`, resolved.grams)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resolved.grams, resolved.source, foodId])

  useEffect(() => {
    if (!ingredientDetail || ingredientDetail.kind !== "leaf") return
    if ((form.getValues(`children.${index}.kcal_100g`) ?? 0) > 0) return
    form.setValue(
      `children.${index}.kcal_100g`,
      ingredientDetail.kcal_100g ?? 0
    )
    form.setValue(
      `children.${index}.protein_100g`,
      ingredientDetail.protein_100g ?? 0
    )
    form.setValue(`children.${index}.fat_100g`, ingredientDetail.fat_100g ?? 0)
    form.setValue(
      `children.${index}.carbs_100g`,
      ingredientDetail.carbs_100g ?? 0
    )
    form.setValue(
      `children.${index}.fiber_100g`,
      ingredientDetail.fiber_100g ?? 0
    )
    form.setValue(
      `children.${index}.sodium_100g`,
      ingredientDetail.sodium_100g ?? 0
    )
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ingredientDetail])

  function selectIngredient(ing: LeafFood) {
    form.setValue(`children.${index}.child_id`, ing.id)
    form.setValue(`children.${index}.child_name`, ing.name)
    form.setValue(`children.${index}.child_kind`, ing.kind)
    form.setValue(`children.${index}.kcal_100g`, ing.kcal_100g ?? 0)
    form.setValue(`children.${index}.protein_100g`, ing.protein_100g ?? 0)
    form.setValue(`children.${index}.fat_100g`, ing.fat_100g ?? 0)
    form.setValue(`children.${index}.carbs_100g`, ing.carbs_100g ?? 0)
    form.setValue(`children.${index}.fiber_100g`, ing.fiber_100g ?? 0)
    form.setValue(`children.${index}.sodium_100g`, ing.sodium_100g ?? 0)
  }

  function handleUnitChange(newUnit: string) {
    const canonical = normalizeUnit(newUnit)
    form.setValue(`children.${index}.unit`, canonical)
    form.setValue(`children.${index}.grams`, 0)
  }

  const needsManualGrams =
    resolved.source === "unresolved" || resolved.source === "manual"
  const gramsEditable = needsManualGrams
  const canAddPortion =
    foodId > 0 &&
    !!resolved.unit &&
    isCountUnit(resolved.unit) &&
    (resolved.source === "unresolved" || resolved.source === "manual")

  // Proactive hint: ingredient is selected, portions have loaded and there are
  // none yet, and the user hasn't already switched to a count unit (which
  // triggers the reactive "Portion hinzufügen" CTA below the row). Skips when
  // the user has dismissed it for this row.
  const showProactivePortionHint =
    foodId > 0 &&
    portionsLoaded &&
    portions.length === 0 &&
    !canAddPortion &&
    !proactiveHintDismissed

  function openProactivePortionDialog() {
    // Pre-populate the inline dialog with the canonical count unit so the
    // existing saveInlinePortion flow applies — no new dialog to build.
    form.setValue(`children.${index}.unit`, "piece")
    form.setValue(`children.${index}.grams`, 0)
    setAddPortionOpen(true)
    setProactiveHintDismissed(true)
  }

  async function saveInlinePortion() {
    const g = Number(addPortionGrams)
    if (!Number.isFinite(g) || g <= 0) return
    await upsertPortion.mutateAsync({
      foodId,
      data: { unit: resolved.unit, grams: g },
    })
    form.setValue(`children.${index}.grams`, 0)
    setAddPortionOpen(false)
    setAddPortionGrams("")
  }

  return (
    <div
      className="space-y-3 rounded-xl border border-outline-variant/30 bg-surface-container/40 p-3"
      data-testid={`ingredient-row-${index}`}
    >
      <div className="flex items-start gap-2">
        <div className="flex-1">
          <IngredientCombobox
            value={foodId}
            selectedName={ingredientName}
            onSelect={selectIngredient}
            testId={`ingredient-row-${index}-combobox`}
          />
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          onClick={() => onRemove(index)}
          aria-label={t("common.delete")}
        >
          <Trash2 className="size-4" />
        </Button>
      </div>

      {showProactivePortionHint && (
        <div
          className="flex flex-wrap items-start gap-2 rounded-lg border border-primary-fixed/60 bg-primary-fixed/30 px-3 py-2 text-xs text-on-primary-fixed"
          data-testid={`ingredient-row-${index}-proactive-hint`}
        >
          <Lightbulb className="mt-0.5 size-3.5 shrink-0" aria-hidden />
          <p className="flex-1">{t("component.portion_hint_proactive")}</p>
          <button
            type="button"
            className="font-semibold text-primary underline-offset-2 hover:underline"
            onClick={openProactivePortionDialog}
            data-testid={`ingredient-row-${index}-proactive-add`}
          >
            {t("component.portion_hint_add")}
          </button>
          <button
            type="button"
            aria-label={t("common.delete")}
            className="text-on-primary-fixed/70 hover:text-on-primary-fixed"
            onClick={() => setProactiveHintDismissed(true)}
          >
            ×
          </button>
        </div>
      )}

      {foodId > 0 && (
        <>
          <div className="grid grid-cols-[minmax(72px,0.7fr)_minmax(0,1.6fr)_minmax(96px,0.9fr)] gap-2">
            <FormField
              control={control}
              name={`children.${index}.amount`}
              render={({ field }) => (
                <FormItem>
                  <FormLabel className="text-xs">
                    {t("component.amount")}
                  </FormLabel>
                  <FormControl>
                    <Input
                      type="number"
                      step="any"
                      min="0.1"
                      className="tabular-nums"
                      {...field}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
            <FormItem>
              <FormLabel className="text-xs">{t("component.unit")}</FormLabel>
              <UnitSelect
                value={unit}
                onValueChange={handleUnitChange}
                portions={portions}
                testId={`ingredient-row-${index}-unit`}
              />
            </FormItem>
            <FormField
              control={control}
              name={`children.${index}.grams`}
              render={({ field }) => (
                <FormItem>
                  <FormLabel className="text-xs">
                    {t("component.grams")}
                  </FormLabel>
                  <FormControl>
                    <Input
                      type="number"
                      step="any"
                      min="0"
                      {...field}
                      disabled={!gramsEditable}
                      className={cn(
                        "tabular-nums",
                        !gramsEditable &&
                          "bg-surface-container-high/40 text-on-surface/80"
                      )}
                      data-testid={`ingredient-row-${index}-grams`}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
          </div>
          <div className="flex items-center justify-between gap-2 text-xs">
            <GramsSourceBadge
              source={resolved.source}
              testId={`ingredient-row-${index}-badge`}
            />
            {canAddPortion && (
              <button
                type="button"
                className="text-primary underline-offset-2 hover:underline"
                onClick={() => setAddPortionOpen(true)}
                data-testid={`ingredient-row-${index}-add-portion`}
              >
                {t("component.add_portion_for_unit", {
                  unit: t(`unit.${resolved.unit}.name`, {
                    defaultValue: resolved.unit,
                  }),
                })}
              </button>
            )}
          </div>
        </>
      )}

      <Dialog open={addPortionOpen} onOpenChange={setAddPortionOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {t("component.add_portion_title", {
                unit: t(`unit.${resolved.unit}.name`, {
                  defaultValue: resolved.unit,
                }),
              })}
            </DialogTitle>
            <DialogDescription>
              {t("component.add_portion_body", {
                unit: t(`unit.${resolved.unit}.name`, {
                  defaultValue: resolved.unit,
                }),
                ingredient: ingredientName,
              })}
            </DialogDescription>
          </DialogHeader>
          <div>
            <FormLabel className="text-xs">{t("component.grams")}</FormLabel>
            <Input
              type="number"
              min="0.1"
              step="any"
              value={addPortionGrams}
              onChange={(e) => setAddPortionGrams(e.target.value)}
              data-testid={`ingredient-row-${index}-add-portion-grams`}
            />
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setAddPortionOpen(false)}
              type="button"
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="button"
              onClick={saveInlinePortion}
              disabled={upsertPortion.isPending || !addPortionGrams}
              data-testid={`ingredient-row-${index}-save-portion`}
            >
              {t("common.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

export const IngredientRow = memo(IngredientRowImpl)
