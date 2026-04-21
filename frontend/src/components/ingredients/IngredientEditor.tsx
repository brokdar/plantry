import { useEffect, useRef, useState } from "react"
import { useForm, useWatch, type Resolver } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { useTranslation } from "react-i18next"
import { useRouter } from "@tanstack/react-router"
import { AlertCircle, Loader2 } from "lucide-react"

import { MacroDistributionBar, MacroTriad } from "@/components/editorial/macros"
import { NutritionPanel } from "@/components/editorial/NutritionPanel"
import { SectionCard } from "@/components/editorial/SectionCard"
import { StickyActionBar } from "@/components/editorial/StickyActionBar"
import { ImageField } from "@/components/images/ImageField"
import { useFetchImageFromUrl, useUploadImage } from "@/lib/queries/images"
import { toastError } from "@/lib/toast"
import { Button } from "@/components/ui/button"
import {
  Form,
  FormField,
  FormItem,
  FormLabel,
  FormControl,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"

import { DeleteIngredientDialog } from "./DeleteIngredientDialog"
import {
  EXTENDED_MACRO_FIELDS,
  ExtendedNutrientFieldSet,
  MINERAL_FIELDS,
  VITAMIN_FIELDS,
} from "./ExtendedNutrientFieldSet"
import { IngredientMetaToolbar } from "./IngredientMetaToolbar"
import { LookupPanel } from "./LookupPanel"
import { MacroFieldSet } from "./MacroFieldSet"
import { PortionsEditor, type StagedPortion } from "./PortionsEditor"

import {
  EXTENDED_NUTRIENT_KEYS,
  type ExtendedNutrientKey,
} from "@/lib/api/ingredients"

import {
  ingredientSchema,
  type IngredientFormValues,
} from "@/lib/schemas/ingredient"
import {
  useCreateIngredient,
  useUpdateIngredient,
} from "@/lib/queries/ingredients"
import { useUpsertPortion } from "@/lib/queries/portions"
import type { Ingredient } from "@/lib/api/ingredients"
import type { LookupCandidate } from "@/lib/api/lookup"
import { ApiError } from "@/lib/api/client"

interface IngredientEditorProps {
  ingredient?: Ingredient
  onSuccess?: () => void
  onDeleted?: () => void
  onCancel?: () => void
}

const MACRO_WATCH_FIELDS = [
  "kcal_100g",
  "protein_100g",
  "fat_100g",
  "carbs_100g",
  "fiber_100g",
  "sodium_100g",
] as const

/**
 * Map an ingredient, lookup candidate, or nothing to a fully populated form
 * value object. Single source of truth for form seeding: add a new nutrient
 * key in one place (EXTENDED_NUTRIENT_KEYS) and every seed path picks it up.
 */
function toFormValues(
  input?: Ingredient | LookupCandidate
): IngredientFormValues {
  const extended = Object.fromEntries(
    EXTENDED_NUTRIENT_KEYS.map((k) => [k, input?.[k] ?? null])
  ) as Record<ExtendedNutrientKey, number | null>

  if (!input) {
    return {
      name: "",
      source: "manual",
      barcode: null,
      off_id: null,
      fdc_id: null,
      kcal_100g: 0,
      protein_100g: 0,
      fat_100g: 0,
      carbs_100g: 0,
      fiber_100g: 0,
      sodium_100g: 0,
      ...extended,
    }
  }

  const isIngredient = "id" in input
  const barcode = input.barcode ?? null
  const off_id = isIngredient
    ? input.off_id
    : input.source === "off"
      ? barcode
      : null
  const fdc_id = isIngredient
    ? input.fdc_id
    : input.fdc_id != null
      ? String(input.fdc_id)
      : null

  return {
    name: input.name,
    source: input.source,
    barcode,
    off_id,
    fdc_id,
    kcal_100g: input.kcal_100g ?? 0,
    protein_100g: input.protein_100g ?? 0,
    fat_100g: input.fat_100g ?? 0,
    carbs_100g: input.carbs_100g ?? 0,
    fiber_100g: input.fiber_100g ?? 0,
    sodium_100g: input.sodium_100g ?? 0,
    ...extended,
  }
}

export function IngredientEditor({
  ingredient,
  onSuccess,
  onDeleted,
  onCancel,
}: IngredientEditorProps) {
  const { t } = useTranslation()
  const router = useRouter()
  const isEdit = !!ingredient

  const form = useForm<IngredientFormValues>({
    resolver: zodResolver(ingredientSchema) as Resolver<IngredientFormValues>,
    defaultValues: toFormValues(ingredient),
  })

  const createMutation = useCreateIngredient()
  const updateMutation = useUpdateIngredient()
  const uploadMutation = useUploadImage()
  const fetchImageMutation = useFetchImageFromUrl()
  const upsertPortionMutation = useUpsertPortion()

  const [stagedImage, setStagedImage] = useState<Blob | null>(null)
  const [stagedPortions, setStagedPortions] = useState<StagedPortion[]>([])

  const isPending = isEdit ? updateMutation.isPending : createMutation.isPending

  const macroValues = useWatch({
    control: form.control,
    name: MACRO_WATCH_FIELDS,
  })
  const [kcal, protein, fat, carbs, fiber, sodium] = macroValues.map(
    (v) => Number(v) || 0
  )
  const hasAnyMacro =
    kcal > 0 || protein > 0 || fat > 0 || carbs > 0 || fiber > 0 || sodium > 0

  const watchedName = useWatch({ control: form.control, name: "name" })
  const nameEmpty = !(watchedName ?? "").trim()

  // Focus the Name input on landing in create mode so keyboard users don't
  // have to Tab out of the app shell to reach the first form field.
  const nameInputRef = useRef<HTMLInputElement | null>(null)
  useEffect(() => {
    if (!isEdit) nameInputRef.current?.focus()
  }, [isEdit])

  async function onSubmit(v: IngredientFormValues) {
    try {
      if (isEdit && ingredient) {
        await updateMutation.mutateAsync({ id: ingredient.id, data: v })
      } else {
        const created = await createMutation.mutateAsync(v)
        if (stagedImage) {
          try {
            await uploadMutation.mutateAsync({
              entityType: "ingredients",
              id: created.id,
              file: stagedImage,
            })
          } catch (err) {
            toastError(err, t)
          }
          setStagedImage(null)
        }
        if (stagedPortions.length > 0) {
          let anyFailed = false
          for (const p of stagedPortions) {
            try {
              await upsertPortionMutation.mutateAsync({
                ingredientId: created.id,
                data: p,
              })
            } catch {
              anyFailed = true
            }
          }
          if (anyFailed) {
            toastError(new ApiError(500, "error.server"), t)
          }
          setStagedPortions([])
        }
      }
      onSuccess?.()
    } catch (err) {
      if (err instanceof ApiError) {
        form.setError("root", { message: t(err.messageKey) })
      }
    }
  }

  async function handleLookupSelect(candidate: LookupCandidate) {
    form.reset(toFormValues(candidate))
    if (!candidate.image_url) return
    try {
      const blob = await fetchImageMutation.mutateAsync({
        url: candidate.image_url,
      })
      setStagedImage(blob)
    } catch {
      // Keep the user's previously staged image — lookup nutrition still applies.
    }
  }

  function handleCancel() {
    if (onCancel) {
      onCancel()
      return
    }
    router.history.back()
  }

  const saveButton = (
    <Button type="submit" disabled={isPending || nameEmpty}>
      {isPending && <Loader2 className="mr-2 size-4 animate-spin" />}
      {t("common.save")}
    </Button>
  )
  const savePrimary =
    nameEmpty && !isPending ? (
      <Tooltip>
        <TooltipTrigger asChild>
          <span tabIndex={0} data-testid="save-disabled-wrapper">
            {saveButton}
          </span>
        </TooltipTrigger>
        <TooltipContent>{t("ingredient.save_disabled_reason")}</TooltipContent>
      </Tooltip>
    ) : (
      saveButton
    )

  return (
    <TooltipProvider delayDuration={200}>
      <Form {...form}>
        <form
          noValidate
          onSubmit={form.handleSubmit(onSubmit)}
          className="space-y-6"
        >
          {form.formState.errors.root && (
            <div
              role="alert"
              className="flex items-start gap-2 rounded-xl border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive"
            >
              <AlertCircle className="mt-0.5 size-4 shrink-0" aria-hidden />
              <p>{form.formState.errors.root.message}</p>
            </div>
          )}

          {isEdit && ingredient && (
            <IngredientMetaToolbar
              ingredient={ingredient}
              disabled={isPending}
              onRefetched={(updated) => form.reset(toFormValues(updated))}
            />
          )}

          {!isEdit && (
            <SectionCard
              title={t("lookup.title")}
              description={t("ingredient.lookup_primary_hint")}
              testId="lookup-primary-banner"
            >
              <LookupPanel onSelect={handleLookupSelect} />
            </SectionCard>
          )}

          <div className="grid grid-cols-1 gap-6 lg:grid-cols-12">
            {/* Left column */}
            <div className="space-y-4 lg:col-span-7">
              <SectionCard title={t("ingredient.section_basic")}>
                <FormField
                  control={form.control}
                  name="name"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t("ingredient.name")}</FormLabel>
                      <FormControl>
                        <Input
                          placeholder={t("ingredient.name_placeholder")}
                          disabled={isPending}
                          {...field}
                          ref={(el) => {
                            field.ref(el)
                            nameInputRef.current = el
                          }}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </SectionCard>

              <SectionCard
                title={t("ingredient.section_macros")}
                description={t("ingredient.section_macros_hint")}
              >
                <MacroFieldSet control={form.control} disabled={isPending} />
                <MacroDistributionBar
                  thickness="md"
                  values={{ protein, carbs, fat }}
                />
                <MacroTriad
                  size="sm"
                  values={{ protein, carbs, fat }}
                  className="text-xs"
                />
              </SectionCard>

              <SectionCard title={t("nutrition.section_extended")}>
                <ExtendedNutrientFieldSet
                  control={form.control}
                  fields={EXTENDED_MACRO_FIELDS}
                  disabled={isPending}
                />
              </SectionCard>

              <SectionCard title={t("nutrition.section_minerals")}>
                <ExtendedNutrientFieldSet
                  control={form.control}
                  fields={MINERAL_FIELDS}
                  disabled={isPending}
                />
              </SectionCard>

              <SectionCard title={t("nutrition.section_vitamins")}>
                <ExtendedNutrientFieldSet
                  control={form.control}
                  fields={VITAMIN_FIELDS}
                  disabled={isPending}
                />
              </SectionCard>

              <SectionCard
                title={t("portion.title")}
                description={
                  isEdit ? undefined : t("ingredient.staged_portions_hint")
                }
              >
                {isEdit && ingredient ? (
                  <PortionsEditor mode="bound" ingredientId={ingredient.id} />
                ) : (
                  <PortionsEditor
                    mode="staged"
                    portions={stagedPortions}
                    onChange={setStagedPortions}
                  />
                )}
              </SectionCard>
            </div>

            {/* Right column */}
            <div className="space-y-4 lg:sticky lg:top-6 lg:col-span-5 lg:self-start">
              <SectionCard
                title={t("image.section_title")}
                description={t("ingredient.section_image_hint")}
              >
                {isEdit && ingredient ? (
                  <ImageField
                    mode="bound"
                    entityType="ingredients"
                    entityId={ingredient.id}
                    currentImagePath={ingredient.image_path}
                    onImageChange={() => {}}
                  />
                ) : (
                  <ImageField
                    mode="staged"
                    stagedBlob={stagedImage}
                    onStagedChange={setStagedImage}
                  />
                )}
              </SectionCard>

              {hasAnyMacro ? (
                <NutritionPanel
                  variant="static"
                  title={t("ingredient.per_100g")}
                  testId="ingredient-nutrition-summary"
                  macros={{ kcal, protein, fat, carbs, fiber, sodium }}
                />
              ) : (
                <SectionCard
                  title={t("ingredient.per_100g")}
                  testId="ingredient-nutrition-summary-empty"
                >
                  <p className="text-sm text-on-surface-variant">
                    {t("ingredient.nutrition_preview_empty")}
                  </p>
                </SectionCard>
              )}
            </div>
          </div>

          <StickyActionBar
            primary={savePrimary}
            secondary={
              <Button type="button" variant="outline" onClick={handleCancel}>
                {t("common.cancel")}
              </Button>
            }
            destructive={
              isEdit && ingredient ? (
                <DeleteIngredientDialog
                  ingredientId={ingredient.id}
                  onDeleted={onDeleted}
                />
              ) : undefined
            }
          />
        </form>
      </Form>
    </TooltipProvider>
  )
}
