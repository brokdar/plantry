import { useEffect, useRef, useState } from "react"
import { useForm, useWatch, type Resolver } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { useTranslation } from "react-i18next"
import { useRouter } from "@tanstack/react-router"
import { AlertCircle, Loader2 } from "lucide-react"

import { MacroLine } from "@/components/editorial/MacroLine"
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
  FormDescription,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
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
import { NoCookSelect } from "@/components/recipes/NoCookSelect"
import { LookupPanel } from "./LookupPanel"
import { MacroFieldSet } from "./MacroFieldSet"
import { PortionsEditor, type StagedPortion } from "./PortionsEditor"

import {
  EXTENDED_NUTRIENT_KEYS,
  type ExtendedNutrientKey,
} from "@/lib/api/foods"

import {
  LEAF_ROLES,
  leafFoodSchema,
  type LeafFoodFormValues,
} from "@/lib/schemas/food"
import {
  useCreateFood,
  useSetNoCook,
  useUpdateFood,
  useUpsertPortion,
} from "@/lib/queries/foods"
import type { LeafFood } from "@/lib/api/foods"
import type { LookupCandidate } from "@/lib/api/lookup"
import { ApiError } from "@/lib/api/client"

interface IngredientEditorProps {
  ingredient?: LeafFood
  onSuccess?: () => void
  onDeleted?: () => void
  onCancel?: () => void
}

/** The role select's "no override" item (a Select item needs a value). */
const NO_ROLE = "none"

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
function toFormValues(input?: LeafFood | LookupCandidate): LeafFoodFormValues {
  const extended = Object.fromEntries(
    EXTENDED_NUTRIENT_KEYS.map((k) => [k, input?.[k] ?? null])
  ) as Record<ExtendedNutrientKey, number | null>

  if (!input) {
    return {
      kind: "leaf",
      name: "",
      role: null,
      no_cook: null,
      source: "manual",
      barcode: null,
      off_id: null,
      fdc_id: null,
      kcal_100g: null,
      protein_100g: null,
      fat_100g: null,
      carbs_100g: null,
      fiber_100g: null,
      sodium_100g: null,
      ...extended,
    }
  }

  const isIngredient = "id" in input
  const barcode = input.barcode ?? null
  const off_id = isIngredient
    ? (input.off_id ?? null)
    : input.source === "off"
      ? barcode
      : null
  const fdc_id = isIngredient
    ? (input.fdc_id ?? null)
    : input.fdc_id != null
      ? String(input.fdc_id)
      : null

  return {
    kind: "leaf",
    name: input.name,
    role: isIngredient ? leafRoleOverride(input) : null,
    no_cook: isIngredient ? noCookOverride(input) : null,
    source: input.source ?? "manual",
    barcode,
    off_id,
    fdc_id,
    kcal_100g: input.kcal_100g ?? null,
    protein_100g: input.protein_100g ?? null,
    fat_100g: input.fat_100g ?? null,
    carbs_100g: input.carbs_100g ?? null,
    fiber_100g: input.fiber_100g ?? null,
    sodium_100g: input.sodium_100g ?? null,
    ...extended,
  }
}

/** The ingredient's own role, or null when it follows the store section. */
function leafRoleOverride(f: LeafFood): LeafFoodFormValues["role"] {
  return f.role_source === "user" && f.role
    ? (f.role as LeafFoodFormValues["role"])
    : null
}

/** The ingredient's own "needs no cooking" flag, or null when automatic. */
function noCookOverride(f: LeafFood): boolean | null {
  return f.no_cook_source === "user" ? f.no_cook : null
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
  // The role the store section gives while no override is set.
  const derivedRole =
    ingredient?.role_source === "derived" ? ingredient.role : null

  const form = useForm<LeafFoodFormValues>({
    resolver: zodResolver(leafFoodSchema) as Resolver<LeafFoodFormValues>,
    defaultValues: toFormValues(ingredient),
  })

  const createMutation = useCreateFood()
  const updateMutation = useUpdateFood()
  const uploadMutation = useUploadImage()
  const fetchImageMutation = useFetchImageFromUrl()
  const upsertPortionMutation = useUpsertPortion()
  const noCookMutation = useSetNoCook()

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

  async function onSubmit(values: LeafFoodFormValues) {
    // no_cook has its own endpoint; the food payload carries the role only.
    const { no_cook: noCook, ...v } = values
    try {
      if (isEdit && ingredient) {
        await updateMutation.mutateAsync({ id: ingredient.id, data: v })
        if (noCook !== noCookOverride(ingredient)) {
          await noCookMutation.mutateAsync({ id: ingredient.id, value: noCook })
        }
      } else {
        const created = await createMutation.mutateAsync(v)
        if (noCook !== null) {
          // The ingredient exists now: a failed override is toasted by the
          // mutation and must not stop the save (a retry would duplicate it).
          await noCookMutation
            .mutateAsync({ id: created.id, value: noCook })
            .catch(() => undefined)
        }
        if (stagedImage) {
          try {
            await uploadMutation.mutateAsync({
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
                foodId: created.id,
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

              <SectionCard title={t("ingredient.section_planning")}>
                <FormField
                  control={form.control}
                  name="role"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t("ingredient.role")}</FormLabel>
                      <Select
                        value={field.value ?? NO_ROLE}
                        onValueChange={(v) =>
                          field.onChange(v === NO_ROLE ? null : v)
                        }
                        disabled={isPending}
                      >
                        <FormControl>
                          <SelectTrigger data-testid="ingredient-role">
                            <SelectValue
                              placeholder={t("ingredient.role_auto")}
                            />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value={NO_ROLE}>
                            {derivedRole
                              ? `${t("ingredient.role_auto")} (${t(`component.role_${derivedRole}`)})`
                              : t("ingredient.role_auto")}
                          </SelectItem>
                          {LEAF_ROLES.map((role) => (
                            <SelectItem key={role} value={role}>
                              {t(`component.role_${role}`)}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormDescription>
                        {t("ingredient.role_hint")}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name="no_cook"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t("no_cook.label")}</FormLabel>
                      <NoCookSelect
                        value={field.value}
                        effective={
                          ingredient?.no_cook_source === "user"
                            ? undefined
                            : ingredient?.no_cook
                        }
                        onChange={field.onChange}
                        disabled={isPending}
                        testId="ingredient-no-cook"
                      />
                      <FormDescription>{t("no_cook.hint")}</FormDescription>
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
                <MacroLine
                  kcal={kcal}
                  protein={protein}
                  fat={fat}
                  carbs={carbs}
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
                  <PortionsEditor mode="bound" foodId={ingredient.id} />
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
                  <p className="text-sm text-ink-muted">
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
                  foodId={ingredient.id}
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
