import { useEffect, useState } from "react"
import {
  useForm,
  useFieldArray,
  useWatch,
  type Resolver,
  type UseFormReturn,
} from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { useTranslation } from "react-i18next"
import { Link, useNavigate } from "@tanstack/react-router"
import { GitBranch, Loader2, Plus, Trash2, X } from "lucide-react"

import {
  FoodPlaceholder,
  type FoodPlaceholderCategory,
} from "@/components/editorial/FoodPlaceholder"
import { imageURL } from "@/lib/image-url"

import { NutritionPanel } from "@/components/editorial/NutritionPanel"
import { SectionCard } from "@/components/editorial/SectionCard"
import { StickyActionBar } from "@/components/editorial/StickyActionBar"
import { ImageField } from "@/components/images/ImageField"
import { useUploadImage } from "@/lib/queries/images"
import { toastError } from "@/lib/toast"
import { Badge } from "@/components/ui/badge"
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
  Form,
  FormField,
  FormItem,
  FormLabel,
  FormControl,
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
import { Textarea } from "@/components/ui/textarea"

import { IngredientCombobox } from "./IngredientCombobox"
import { UnitSelect } from "@/components/ingredients/UnitSelect"
import { cn } from "@/lib/utils"

import {
  componentSchema,
  COMPONENT_ROLES,
  type ComponentFormValues,
} from "@/lib/schemas/component"
import {
  useCreateComponent,
  useUpdateComponent,
  useDeleteComponent,
  useCreateVariant,
  useVariants,
} from "@/lib/queries/components"
import { useIngredient } from "@/lib/queries/ingredients"
import { usePortions, useUpsertPortion } from "@/lib/queries/portions"
import { fromIngredients, type IngredientInput } from "@/lib/domain/nutrition"
import {
  isCountUnit,
  normalizeUnit,
  resolveGrams,
  type GramsSource,
} from "@/lib/domain/units"
import type { Component } from "@/lib/api/components"
import type { Ingredient } from "@/lib/api/ingredients"
import { ApiError } from "@/lib/api/client"

interface ComponentEditorProps {
  component?: Component
  onSuccess?: () => void
  onDeleted?: () => void
}

export function ComponentEditor({
  component,
  onSuccess,
  onDeleted,
}: ComponentEditorProps) {
  const { t } = useTranslation()
  const isEdit = !!component

  const form = useForm<ComponentFormValues>({
    resolver: zodResolver(componentSchema) as Resolver<ComponentFormValues>,
    defaultValues: component
      ? {
          name: component.name,
          role: component.role as ComponentFormValues["role"],
          reference_portions: component.reference_portions,
          prep_minutes: component.prep_minutes ?? 0,
          cook_minutes: component.cook_minutes ?? 0,
          notes: component.notes,
          ingredients: component.ingredients.map((ci) => ({
            ingredient_id: ci.ingredient_id,
            ingredient_name: ci.ingredient_name,
            amount: ci.amount,
            unit: ci.unit,
            grams: ci.grams,
            sort_order: ci.sort_order,
          })),
          instructions: component.instructions.map((inst) => ({
            step_number: inst.step_number,
            text: inst.text,
          })),
          tags: component.tags,
        }
      : {
          name: "",
          role: "main" as const,
          reference_portions: 1,
          prep_minutes: 0,
          cook_minutes: 0,
          notes: null,
          ingredients: [],
          instructions: [],
          tags: [],
        },
  })

  const {
    fields: ingredientFields,
    append: appendIngredient,
    remove: removeIngredient,
  } = useFieldArray({ control: form.control, name: "ingredients" })

  const {
    fields: instructionFields,
    append: appendInstruction,
    remove: removeInstruction,
  } = useFieldArray({ control: form.control, name: "instructions" })

  const createMutation = useCreateComponent()
  const updateMutation = useUpdateComponent()
  const deleteMutation = useDeleteComponent()
  const createVariant = useCreateVariant()
  const uploadMutation = useUploadImage()
  const navigate = useNavigate()
  const { data: variantsData } = useVariants(component?.id ?? 0)

  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [stagedImage, setStagedImage] = useState<Blob | null>(null)

  const watchedIngredients = useWatch({
    control: form.control,
    name: "ingredients",
  })
  const watchedPortions = useWatch({
    control: form.control,
    name: "reference_portions",
  })
  const watchedPrep = useWatch({ control: form.control, name: "prep_minutes" })
  const watchedCook = useWatch({ control: form.control, name: "cook_minutes" })
  const totalTime = (Number(watchedPrep) || 0) + (Number(watchedCook) || 0)

  async function onSubmit(values: ComponentFormValues) {
    // Guard: count units without a portion and no manual grams can't be
    // resolved to mass → nutrition totals would silently underreport.
    const unresolved = values.ingredients.find((ci) => {
      if (!ci.ingredient_id) return false
      const u = normalizeUnit(ci.unit)
      if (!u) return true
      if (isCountUnit(u) && !(ci.grams > 0)) return true
      return false
    })
    if (unresolved) {
      form.setError("root", {
        message: t("component.error_unresolved_unit", {
          unit: t(`unit.${normalizeUnit(unresolved.unit)}.name`, {
            defaultValue: unresolved.unit,
          }),
        }),
      })
      return
    }

    const input = {
      name: values.name,
      role: values.role,
      reference_portions: values.reference_portions,
      prep_minutes: values.prep_minutes,
      cook_minutes: values.cook_minutes,
      notes: values.notes,
      ingredients: values.ingredients.map((ci, idx) => ({
        ingredient_id: ci.ingredient_id,
        amount: ci.amount,
        unit: ci.unit,
        grams: ci.grams,
        sort_order: idx,
      })),
      instructions: values.instructions.map((inst, idx) => ({
        step_number: idx + 1,
        text: inst.text,
      })),
      tags: values.tags,
    }

    try {
      if (isEdit && component) {
        await updateMutation.mutateAsync({ id: component.id, data: input })
      } else {
        const created = await createMutation.mutateAsync(input)
        if (stagedImage) {
          try {
            await uploadMutation.mutateAsync({
              entityType: "components",
              id: created.id,
              file: stagedImage,
            })
          } catch (err) {
            toastError(err, t)
          }
          setStagedImage(null)
        }
      }
      onSuccess?.()
    } catch (err) {
      if (err instanceof ApiError) {
        form.setError("root", { message: t(err.messageKey) })
      }
    }
  }

  function confirmDelete() {
    if (!component) return
    setDeleteError(null)
    deleteMutation.mutate(component.id, {
      onSuccess: () => {
        setDeleteOpen(false)
        onDeleted?.()
      },
      onError: (err: unknown) => {
        const key = err instanceof Error ? err.message : "error.server"
        setDeleteError(t(key))
      },
    })
  }

  const [tagInput, setTagInput] = useState("")
  const watchedTags = form.watch("tags")

  function addTag() {
    const tag = tagInput.trim()
    if (tag && !watchedTags.includes(tag)) {
      form.setValue("tags", [...watchedTags, tag])
    }
    setTagInput("")
  }

  function removeTag(tag: string) {
    form.setValue(
      "tags",
      watchedTags.filter((t) => t !== tag)
    )
  }

  const isPending = createMutation.isPending || updateMutation.isPending
  const perPortionMacros = computePerPortionMacros(
    watchedIngredients,
    watchedPortions
  )

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6">
        {form.formState.errors.root && (
          <p className="text-sm text-destructive">
            {form.formState.errors.root.message}
          </p>
        )}

        {isEdit && component && (
          <div className="flex justify-end">
            <Button
              type="button"
              variant="outline"
              disabled={createVariant.isPending}
              onClick={() =>
                createVariant.mutate(component.id, {
                  onSuccess: (variant) => {
                    void navigate({
                      to: "/components/$id/edit",
                      params: { id: String(variant.id) },
                    })
                  },
                })
              }
              data-testid="component-create-variant"
            >
              <GitBranch className="mr-1.5 size-4" aria-hidden />
              {t("component.create_variant")}
            </Button>
          </div>
        )}

        <div className="grid grid-cols-1 gap-6 lg:grid-cols-12">
          {/* Left column */}
          <div className="space-y-4 lg:col-span-8">
            <SectionCard title={t("component.section_identity")}>
              <div className="grid gap-4 sm:grid-cols-3">
                <FormField
                  control={form.control}
                  name="name"
                  render={({ field }) => (
                    <FormItem className="sm:col-span-2">
                      <FormLabel>{t("component.name")}</FormLabel>
                      <FormControl>
                        <Input
                          placeholder={t("component.name_placeholder")}
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name="reference_portions"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t("component.reference_portions")}</FormLabel>
                      <FormControl>
                        <Input type="number" step="0.5" min="0.5" {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
              <FormField
                control={form.control}
                name="role"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("component.role")}</FormLabel>
                    <Select
                      onValueChange={field.onChange}
                      defaultValue={field.value}
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue
                            placeholder={t("component.role_placeholder")}
                          />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        {COMPONENT_ROLES.map((role) => (
                          <SelectItem key={role} value={role}>
                            {t(`component.role_${role}`)}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </SectionCard>

            <SectionCard title={t("component.cooking_time")}>
              <div className="grid grid-cols-2 gap-4">
                <FormField
                  control={form.control}
                  name="prep_minutes"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t("component.prep_minutes")}</FormLabel>
                      <FormControl>
                        <Input type="number" min="0" {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name="cook_minutes"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t("component.cook_minutes")}</FormLabel>
                      <FormControl>
                        <Input type="number" min="0" {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
              {totalTime > 0 && (
                <p className="text-xs tracking-wider text-on-surface-variant uppercase">
                  {t("component.cooking_time_total", { minutes: totalTime })}
                </p>
              )}
            </SectionCard>

            <SectionCard title={t("component.tags")}>
              {watchedTags.length > 0 && (
                <div className="flex flex-wrap gap-2">
                  {watchedTags.map((tag) => (
                    <Badge key={tag} variant="secondary" className="gap-1">
                      {tag}
                      <button
                        type="button"
                        onClick={() => removeTag(tag)}
                        className="ml-1 rounded-full hover:bg-muted"
                        aria-label={t("common.delete")}
                      >
                        <X className="size-3" />
                      </button>
                    </Badge>
                  ))}
                </div>
              )}
              <div className="flex gap-2">
                <Input
                  placeholder={t("component.tag_placeholder")}
                  value={tagInput}
                  onChange={(e) => setTagInput(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault()
                      addTag()
                    }
                  }}
                  className="max-w-xs"
                />
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={addTag}
                >
                  {t("component.add_tag")}
                </Button>
              </div>
            </SectionCard>

            <SectionCard
              title={t("component.instructions")}
              actions={
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() =>
                    appendInstruction({
                      step_number: instructionFields.length + 1,
                      text: "",
                    })
                  }
                  data-testid="add-instruction"
                >
                  <Plus className="mr-1 size-4" />
                  {t("component.add_instruction")}
                </Button>
              }
            >
              {instructionFields.length === 0 ? (
                <p className="rounded-xl border border-dashed border-outline-variant/40 px-6 py-10 text-center text-sm text-on-surface-variant">
                  {t("component.instructions_empty")}
                </p>
              ) : (
                <ol className="space-y-3">
                  {instructionFields.map((field, index) => (
                    <li key={field.id} className="flex items-start gap-3">
                      <span className="text-on-primary-container mt-2 flex size-7 shrink-0 items-center justify-center rounded-full bg-primary-container font-heading text-xs font-bold">
                        {index + 1}
                      </span>
                      <FormField
                        control={form.control}
                        name={`instructions.${index}.text`}
                        render={({ field }) => (
                          <FormItem className="flex-1">
                            <FormControl>
                              <Textarea
                                placeholder={t(
                                  "component.instruction_placeholder"
                                )}
                                rows={2}
                                {...field}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        className="mt-1.5"
                        onClick={() => removeInstruction(index)}
                        aria-label={t("common.delete")}
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    </li>
                  ))}
                </ol>
              )}
            </SectionCard>

            <SectionCard
              title={t("component.ingredients")}
              actions={
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() =>
                    appendIngredient({
                      ingredient_id: 0,
                      ingredient_name: "",
                      amount: 100,
                      unit: "g",
                      grams: 100,
                      sort_order: ingredientFields.length,
                    })
                  }
                  data-testid="add-ingredient"
                >
                  <Plus className="mr-1 size-4" />
                  {t("component.add_ingredient")}
                </Button>
              }
            >
              {ingredientFields.length === 0 ? (
                <p className="rounded-xl border border-dashed border-outline-variant/40 px-6 py-10 text-center text-sm text-on-surface-variant">
                  {t("component.ingredients_empty")}
                </p>
              ) : (
                <div className="space-y-3">
                  {ingredientFields.map((field, index) => (
                    <IngredientRow
                      key={field.id}
                      index={index}
                      form={form}
                      onRemove={() => removeIngredient(index)}
                    />
                  ))}
                </div>
              )}
            </SectionCard>

            <SectionCard title={t("component.notes")}>
              <FormField
                control={form.control}
                name="notes"
                render={({ field }) => (
                  <FormItem>
                    <FormControl>
                      <Textarea
                        placeholder={t("component.notes_placeholder")}
                        rows={4}
                        {...field}
                        value={field.value ?? ""}
                        onChange={(e) => field.onChange(e.target.value || null)}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </SectionCard>
          </div>

          {/* Right column */}
          <div className="space-y-4 lg:sticky lg:top-6 lg:col-span-4 lg:self-start">
            <SectionCard
              title={t("image.section_title")}
              description={t("component.section_image_hint")}
            >
              {isEdit && component ? (
                <ImageField
                  mode="bound"
                  entityType="components"
                  entityId={component.id}
                  currentImagePath={component.image_path}
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

            {perPortionMacros ? (
              <NutritionPanel
                macros={perPortionMacros}
                referencePortions={watchedPortions}
              />
            ) : (
              <SectionCard title={t("component.nutrition")}>
                <p className="text-sm text-on-surface-variant">
                  {t("component.nutrition_empty")}
                </p>
              </SectionCard>
            )}

            {isEdit && variantsData && variantsData.items.length > 0 && (
              <SectionCard
                title={t("component.other_variants")}
                testId="component-variants-section"
              >
                <div className="-mx-2 flex gap-3 overflow-x-auto px-2 pb-1">
                  {variantsData.items.map((variant) => (
                    <Link
                      key={variant.id}
                      to="/components/$id/edit"
                      params={{ id: String(variant.id) }}
                      className="editorial-shadow group flex w-40 shrink-0 flex-col overflow-hidden rounded-xl bg-surface-container-lowest transition-all duration-200 hover:-translate-y-0.5"
                      data-testid={`variant-card-${variant.id}`}
                    >
                      <div className="aspect-[4/3] overflow-hidden bg-surface-container-high">
                        {variant.image_path ? (
                          <img
                            src={imageURL(
                              variant.image_path,
                              variant.updated_at
                            )}
                            alt=""
                            className="h-full w-full object-cover transition-transform duration-500 group-hover:scale-105"
                            loading="lazy"
                          />
                        ) : (
                          <FoodPlaceholder
                            category={
                              variant.role as
                                | FoodPlaceholderCategory
                                | undefined
                            }
                            className="h-full w-full"
                            aria-label={variant.name}
                          />
                        )}
                      </div>
                      <div className="flex flex-1 flex-col gap-1 px-3 py-3">
                        <p className="truncate text-sm font-medium text-on-surface">
                          {variant.name}
                        </p>
                        <Badge
                          variant="secondary"
                          className="w-fit text-[10px]"
                        >
                          {t(`component.role_${variant.role}`)}
                        </Badge>
                      </div>
                    </Link>
                  ))}
                </div>
              </SectionCard>
            )}
          </div>
        </div>

        <StickyActionBar
          primary={
            <Button type="submit" disabled={isPending}>
              {isPending && <Loader2 className="mr-2 size-4 animate-spin" />}
              {t("common.save")}
            </Button>
          }
          secondary={
            <Button variant="outline" asChild>
              <Link to="/components">{t("common.cancel")}</Link>
            </Button>
          }
          destructive={
            isEdit ? (
              <Button
                type="button"
                variant="ghost"
                onClick={() => setDeleteOpen(true)}
                className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                data-testid="component-delete"
              >
                <Trash2 className="mr-1.5 size-4" />
                {t("common.delete")}
              </Button>
            ) : undefined
          }
        />
      </form>

      <Dialog
        open={deleteOpen}
        onOpenChange={(open) => {
          if (!open) setDeleteError(null)
          setDeleteOpen(open)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("component.delete_confirm_title")}</DialogTitle>
            <DialogDescription>
              {t("component.delete_confirm_body")}
            </DialogDescription>
          </DialogHeader>
          {deleteError && (
            <p className="px-1 text-sm text-destructive">{deleteError}</p>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={confirmDelete}
              disabled={deleteMutation.isPending}
              data-testid="confirm-delete"
            >
              {t("common.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Form>
  )
}

function computePerPortionMacros(
  ingredients: ComponentFormValues["ingredients"],
  referencePortions: number
) {
  const valid = ingredients.filter((i) => i.grams > 0)
  if (valid.length === 0) return null
  const inputs: IngredientInput[] = valid.map((i) => ({
    per_100g: {
      kcal: i.kcal_100g ?? 0,
      protein: i.protein_100g ?? 0,
      fat: i.fat_100g ?? 0,
      carbs: i.carbs_100g ?? 0,
      fiber: i.fiber_100g ?? 0,
      sodium: i.sodium_100g ?? 0,
    },
    grams: i.grams,
  }))
  const total = fromIngredients(inputs)
  const portions = referencePortions > 0 ? referencePortions : 1
  return {
    kcal: total.kcal / portions,
    protein: total.protein / portions,
    fat: total.fat / portions,
    carbs: total.carbs / portions,
    fiber: total.fiber / portions,
    sodium: total.sodium / portions,
  }
}

function IngredientRow({
  index,
  form,
  onRemove,
}: {
  index: number
  form: UseFormReturn<ComponentFormValues>
  onRemove: () => void
}) {
  const { t } = useTranslation()

  const ingredientId = form.watch(`ingredients.${index}.ingredient_id`)
  const ingredientName = form.watch(`ingredients.${index}.ingredient_name`)
  const unit = form.watch(`ingredients.${index}.unit`)
  const amount = Number(form.watch(`ingredients.${index}.amount`)) || 0
  const currentGrams = Number(form.watch(`ingredients.${index}.grams`)) || 0

  const { data: portions } = usePortions(ingredientId)
  const { data: ingredientDetail } = useIngredient(ingredientId)
  const upsertPortion = useUpsertPortion()

  const [addPortionOpen, setAddPortionOpen] = useState(false)
  const [addPortionGrams, setAddPortionGrams] = useState("")

  const resolved = resolveGrams(amount, unit, portions ?? [], currentGrams)

  // Whenever amount/unit/portions change and the resolution is auto (not
  // "manual"), write the computed grams back into the form. We avoid writing
  // when source is "manual" or "unresolved" so the user can override or keep
  // typing without losing their value.
  useEffect(() => {
    if (!ingredientId) return
    if (resolved.source === "manual" || resolved.source === "unresolved") return
    if (Math.abs(resolved.grams - currentGrams) < 0.001) return
    form.setValue(`ingredients.${index}.grams`, resolved.grams)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resolved.grams, resolved.source, ingredientId])

  useEffect(() => {
    if (!ingredientDetail) return
    if ((form.getValues(`ingredients.${index}.kcal_100g`) ?? 0) > 0) return
    form.setValue(`ingredients.${index}.kcal_100g`, ingredientDetail.kcal_100g)
    form.setValue(
      `ingredients.${index}.protein_100g`,
      ingredientDetail.protein_100g
    )
    form.setValue(`ingredients.${index}.fat_100g`, ingredientDetail.fat_100g)
    form.setValue(
      `ingredients.${index}.carbs_100g`,
      ingredientDetail.carbs_100g
    )
    form.setValue(
      `ingredients.${index}.fiber_100g`,
      ingredientDetail.fiber_100g
    )
    form.setValue(
      `ingredients.${index}.sodium_100g`,
      ingredientDetail.sodium_100g
    )
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ingredientDetail])

  function selectIngredient(ing: Ingredient) {
    form.setValue(`ingredients.${index}.ingredient_id`, ing.id)
    form.setValue(`ingredients.${index}.ingredient_name`, ing.name)
    form.setValue(`ingredients.${index}.kcal_100g`, ing.kcal_100g)
    form.setValue(`ingredients.${index}.protein_100g`, ing.protein_100g)
    form.setValue(`ingredients.${index}.fat_100g`, ing.fat_100g)
    form.setValue(`ingredients.${index}.carbs_100g`, ing.carbs_100g)
    form.setValue(`ingredients.${index}.fiber_100g`, ing.fiber_100g)
    form.setValue(`ingredients.${index}.sodium_100g`, ing.sodium_100g)
  }

  function handleUnitChange(newUnit: string) {
    const canonical = normalizeUnit(newUnit)
    form.setValue(`ingredients.${index}.unit`, canonical)
    // Clear grams so the next resolve pass recomputes from scratch (user's
    // previous manual override no longer applies to the new unit).
    form.setValue(`ingredients.${index}.grams`, 0)
  }

  const needsManualGrams =
    resolved.source === "unresolved" || resolved.source === "manual"
  const gramsEditable = needsManualGrams
  // Only surface the CTA when there is no conversion yet for this unit —
  // once a portion or universal default kicks in, the CTA is noise.
  const canAddPortion =
    ingredientId > 0 &&
    !!resolved.unit &&
    isCountUnit(resolved.unit) &&
    (resolved.source === "unresolved" || resolved.source === "manual")

  async function saveInlinePortion() {
    const g = Number(addPortionGrams)
    if (!Number.isFinite(g) || g <= 0) return
    await upsertPortion.mutateAsync({
      ingredientId,
      data: { unit: resolved.unit, grams: g },
    })
    // Clear any manual grams override so the new portion takes over.
    form.setValue(`ingredients.${index}.grams`, 0)
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
            value={ingredientId}
            selectedName={ingredientName}
            onSelect={selectIngredient}
            testId={`ingredient-row-${index}-combobox`}
          />
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          onClick={onRemove}
          aria-label={t("common.delete")}
        >
          <Trash2 className="size-4" />
        </Button>
      </div>

      {ingredientId > 0 && (
        <>
          <div className="grid grid-cols-[minmax(72px,0.7fr)_minmax(0,1.6fr)_minmax(96px,0.9fr)] gap-2">
            <FormField
              control={form.control}
              name={`ingredients.${index}.amount`}
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
                portions={portions ?? []}
                testId={`ingredient-row-${index}-unit`}
              />
            </FormItem>
            <FormField
              control={form.control}
              name={`ingredients.${index}.grams`}
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

function GramsSourceBadge({
  source,
  testId,
}: {
  source: GramsSource
  testId?: string
}) {
  const { t } = useTranslation()
  const variant: Record<GramsSource, "secondary" | "outline" | "destructive"> =
    {
      direct: "secondary",
      portion: "secondary",
      default: "secondary",
      fallback: "outline",
      manual: "outline",
      unresolved: "destructive",
    }
  const labelKey: Record<GramsSource, string> = {
    direct: "component.grams_source.exact",
    portion: "component.grams_source.exact",
    default: "component.grams_source.exact",
    fallback: "component.grams_source.approx",
    manual: "component.grams_source.manual",
    unresolved: "component.grams_source.required",
  }
  return (
    <Badge
      variant={variant[source]}
      className="text-[10px] tracking-wide uppercase"
      data-testid={testId}
      data-source={source}
    >
      {t(labelKey[source])}
    </Badge>
  )
}
