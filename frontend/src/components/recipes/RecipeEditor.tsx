import { useCallback, useMemo, useState } from "react"
import {
  useForm,
  useFieldArray,
  useFormContext,
  useWatch,
  type Resolver,
} from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { useTranslation } from "react-i18next"
import { Link } from "@tanstack/react-router"
import { Loader2, Plus, Trash2, TriangleAlert, X } from "lucide-react"

import {
  FoodPlaceholder,
  type FoodPlaceholderCategory,
} from "@/components/editorial/FoodPlaceholder"
import { imageURL } from "@/lib/image-url"

import { NutritionPanel } from "@/components/editorial/NutritionPanel"
import { SectionCard } from "@/components/editorial/SectionCard"
import { useSettingField } from "@/components/settings/setting-field"
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
import { Textarea } from "@/components/ui/textarea"

import { cn } from "@/lib/utils"

import {
  composedFoodSchema,
  FOOD_ROLES,
  type ComposedFoodFormValues,
} from "@/lib/schemas/food"
import {
  useFoods,
  useCreateFood,
  useUpdateFood,
  useDeleteFood,
  useFoodNutrition,
  useSetDietClass,
  useVariants,
} from "@/lib/queries/foods"
import { fromIngredients, type IngredientInput } from "@/lib/domain/nutrition"
import { isCountUnit, normalizeUnit } from "@/lib/domain/units"
import type { ComposedFood } from "@/lib/api/foods"
import { ApiError } from "@/lib/api/client"

import { DietClassSelect } from "./DietClassSelect"
import { IngredientRow } from "./IngredientRow"
import { NutritionCompletenessWarning } from "./NutritionCompletenessWarning"

interface RecipeEditorProps {
  component?: ComposedFood
  onSuccess?: () => void
  onDeleted?: () => void
}

export function RecipeEditor({
  component,
  onSuccess,
  onDeleted,
}: RecipeEditorProps) {
  const { t } = useTranslation()
  const isEdit = !!component

  const form = useForm<ComposedFoodFormValues>({
    resolver: zodResolver(
      composedFoodSchema
    ) as Resolver<ComposedFoodFormValues>,
    defaultValues: component
      ? {
          kind: "composed" as const,
          name: component.name,
          role: (component.role ?? "main") as ComposedFoodFormValues["role"],
          reference_portions: component.reference_portions ?? 1,
          prep_minutes: component.prep_minutes ?? null,
          cook_minutes: component.cook_minutes ?? null,
          keep_days: component.keep_days ?? null,
          freezable: component.freezable ?? null,
          diet_class: dietOverride(component),
          notes: component.notes,
          source_url: component.source_url ?? "",
          description: component.description ?? null,
          children: (component.children ?? []).map((ci) => ({
            child_id: ci.child_id,
            child_name: ci.child_name,
            child_kind: ci.child_kind,
            amount: ci.amount,
            unit: ci.unit,
            grams: ci.grams,
            sort_order: ci.sort_order,
          })),
          instructions: (component.instructions ?? []).map((inst) => ({
            step_number: inst.step_number,
            text: inst.text,
          })),
          tags: component.tags ?? [],
        }
      : {
          kind: "composed" as const,
          name: "",
          role: "main" as const,
          reference_portions: 1,
          prep_minutes: null,
          cook_minutes: null,
          keep_days: null,
          freezable: null,
          diet_class: null,
          notes: null,
          source_url: "",
          description: null,
          children: [],
          instructions: [],
          tags: [],
        },
  })

  const {
    fields: ingredientFields,
    append: appendIngredient,
    remove: removeIngredient,
  } = useFieldArray({ control: form.control, name: "children" })

  const {
    fields: instructionFields,
    append: appendInstruction,
    remove: removeInstruction,
  } = useFieldArray({ control: form.control, name: "instructions" })

  const createMutation = useCreateFood()
  const updateMutation = useUpdateFood()
  const deleteMutation = useDeleteFood()
  const uploadMutation = useUploadImage()
  const dietMutation = useSetDietClass()
  const { data: variantsData } = useVariants(component?.id ?? 0)
  const { data: savedNutrition } = useFoodNutrition(component?.id ?? 0)

  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [stagedImage, setStagedImage] = useState<Blob | null>(null)
  const [pendingImage, setPendingImage] = useState<{
    componentId: number
    blob: Blob
  } | null>(null)
  const [notesOpen, setNotesOpen] = useState(
    !!(component?.notes && component.notes.length > 0)
  )

  const watchedIngredients = useWatch({
    control: form.control,
    name: "children",
  })
  const watchedPortions = useWatch({
    control: form.control,
    name: "reference_portions",
  })
  const watchedPrep = useWatch({ control: form.control, name: "prep_minutes" })
  const watchedCook = useWatch({ control: form.control, name: "cook_minutes" })
  const totalTime = (Number(watchedPrep) || 0) + (Number(watchedCook) || 0)

  async function onSubmit(values: ComposedFoodFormValues) {
    // Guard: count units without a portion and no manual grams can't be
    // resolved to mass → nutrition totals would silently underreport.
    const unresolved = values.children.find((ci) => {
      if (!ci.child_id) return false
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
      kind: "composed" as const,
      name: values.name,
      role: values.role,
      reference_portions: values.reference_portions,
      prep_minutes: values.prep_minutes,
      cook_minutes: values.cook_minutes,
      keep_days: values.keep_days,
      freezable: values.freezable,
      notes: values.notes,
      // "" clears the stored value on update; omitting would keep it.
      source_url: values.source_url ?? "",
      description: values.description ?? "",
      children: values.children.map((ci, idx) => ({
        child_id: ci.child_id,
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
        if (values.diet_class !== dietOverride(component)) {
          await dietMutation.mutateAsync({
            id: component.id,
            dietClass: values.diet_class,
          })
        }
      } else {
        const created = await createMutation.mutateAsync(input)
        if (values.diet_class) {
          await dietMutation.mutateAsync({
            id: created.id,
            dietClass: values.diet_class,
          })
        }
        if (stagedImage) {
          try {
            await uploadMutation.mutateAsync({
              id: created.id,
              file: stagedImage,
            })
            setStagedImage(null)
          } catch (err) {
            setPendingImage({ componentId: created.id, blob: stagedImage })
            setStagedImage(null)
            toastError(err, t)
            return
          }
        }
      }
      onSuccess?.()
    } catch (err) {
      if (err instanceof ApiError) {
        form.setError("root", { message: t(err.messageKey) })
      }
    }
  }

  async function retryPendingImage() {
    if (!pendingImage) return
    try {
      await uploadMutation.mutateAsync({
        id: pendingImage.componentId,
        file: pendingImage.blob,
      })
      setPendingImage(null)
      onSuccess?.()
    } catch (err) {
      toastError(err, t)
    }
  }

  function skipPendingImage() {
    setPendingImage(null)
    onSuccess?.()
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
        const key = err instanceof ApiError ? err.messageKey : "error.server"
        setDeleteError(t(key))
      },
    })
  }

  const [tagInput, setTagInput] = useState("")
  const watchedTags = form.watch("tags")

  function addTag(explicit?: string) {
    const tag = (explicit ?? tagInput).trim()
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
  const perPortionMacros = useMemo(
    () => computePerPortionMacros(watchedIngredients, watchedPortions),
    [watchedIngredients, watchedPortions]
  )

  const handleRemoveIngredient = useCallback(
    (index: number) => {
      removeIngredient(index)
    },
    [removeIngredient]
  )

  // Tag autocomplete: derive the distinct set of tags already used across the
  // catalog. Self-hosted single-user deploy → aggregating client-side from the
  // existing list query is fine; avoids a dedicated backend endpoint.
  const { data: catalogData } = useFoods({ kind: "composed", limit: 500 })
  const existingTagList = useMemo(() => {
    const set = new Set<string>()
    for (const c of (catalogData?.items ?? []) as ComposedFood[]) {
      for (const tag of c.tags ?? []) set.add(tag)
    }
    return Array.from(set).sort((a, b) => a.localeCompare(b))
  }, [catalogData?.items])
  const tagSuggestions = useMemo(() => {
    const q = tagInput.trim().toLowerCase()
    if (!q) return []
    return existingTagList
      .filter(
        (tag) =>
          tag.toLowerCase().includes(q) &&
          !watchedTags.includes(tag) &&
          tag.toLowerCase() !== q
      )
      .slice(0, 6)
  }, [tagInput, existingTagList, watchedTags])

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6">
        {form.formState.errors.root && (
          <p className="text-sm text-destructive">
            {form.formState.errors.root.message}
          </p>
        )}

        {pendingImage && (
          <div
            className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-destructive/40 bg-destructive/5 px-4 py-3 text-sm text-destructive"
            data-testid="pending-image-banner"
            role="alert"
          >
            <p className="flex-1">{t("component.image_upload_failed")}</p>
            <div className="flex items-center gap-2">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={skipPendingImage}
                disabled={uploadMutation.isPending}
              >
                {t("component.image_upload_skip")}
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={retryPendingImage}
                disabled={uploadMutation.isPending}
                data-testid="pending-image-retry"
              >
                {uploadMutation.isPending && (
                  <Loader2 className="mr-2 size-4 animate-spin" />
                )}
                {t("component.image_upload_retry")}
              </Button>
            </div>
          </div>
        )}

        {savedNutrition && !savedNutrition.complete && (
          <NutritionCompletenessWarning missing={savedNutrition.missing} />
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
                        {FOOD_ROLES.map((role) => (
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
              <FormField
                control={form.control}
                name="diet_class"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("diet.label")}</FormLabel>
                    <DietClassSelect
                      value={field.value}
                      effective={component?.diet_class}
                      onChange={field.onChange}
                      label={t("diet.label")}
                      className="w-full md:pointer-fine:min-h-9"
                      testId="recipe-diet-class"
                    />
                    <FormDescription>{t("diet.editor_hint")}</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="source_url"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("component.source_url")}</FormLabel>
                    <FormControl>
                      <Input
                        type="url"
                        inputMode="url"
                        autoComplete="off"
                        placeholder={t("component.source_url_placeholder")}
                        {...field}
                        value={field.value ?? ""}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="description"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("component.description")}</FormLabel>
                    <FormControl>
                      <Textarea
                        rows={3}
                        {...field}
                        value={field.value ?? ""}
                        onChange={(e) => field.onChange(e.target.value || null)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t("component.description_hint")}
                    </FormDescription>
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
                        <Input
                          type="number"
                          min="0"
                          {...field}
                          value={field.value ?? ""}
                        />
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
                        <Input
                          type="number"
                          min="0"
                          {...field}
                          value={field.value ?? ""}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
              {totalTime > 0 && (
                <p className="text-xs tracking-wider text-ink-muted uppercase">
                  {t("component.cooking_time_total", { minutes: totalTime })}
                </p>
              )}
            </SectionCard>

            <StorageFields />

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
              <div className="space-y-2">
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
                    data-testid="tag-input"
                  />
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => addTag()}
                  >
                    {t("component.add_tag")}
                  </Button>
                </div>
                {tagSuggestions.length > 0 && (
                  <div
                    className="flex flex-wrap gap-1.5"
                    data-testid="tag-suggestions"
                  >
                    {tagSuggestions.map((tag) => (
                      <button
                        key={tag}
                        type="button"
                        onClick={() => addTag(tag)}
                        className="rounded-full border border-line/40 bg-panel px-3 py-1 text-xs text-ink-muted transition-colors hover:border-primary/40 hover:bg-panel-sunken/30 hover:text-ink"
                        data-testid={`tag-suggestion-${tag}`}
                      >
                        + {tag}
                      </button>
                    ))}
                  </div>
                )}
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
                <div className="space-y-2">
                  <p
                    className={cn(
                      "rounded-xl border border-dashed px-6 py-10 text-center text-sm",
                      form.formState.errors.instructions
                        ? "border-destructive/50 bg-destructive/5 text-destructive"
                        : "border-line/40 text-ink-muted"
                    )}
                  >
                    {form.formState.errors.instructions?.root?.message ??
                      form.formState.errors.instructions?.message ??
                      t("component.instructions_empty")}
                  </p>
                </div>
              ) : (
                <ol className="space-y-3">
                  {instructionFields.map((field, index) => (
                    <li key={field.id} className="flex items-start gap-3">
                      <span className="mt-2 flex size-7 shrink-0 items-center justify-center rounded-full bg-accent font-heading text-xs font-bold text-on-accent">
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
                      child_id: 0,
                      child_name: "",
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
                <div className="space-y-2">
                  <p
                    className={cn(
                      "rounded-xl border border-dashed px-6 py-10 text-center text-sm",
                      form.formState.errors.children
                        ? "border-destructive/50 bg-destructive/5 text-destructive"
                        : "border-line/40 text-ink-muted"
                    )}
                  >
                    {form.formState.errors.children?.root?.message ??
                      form.formState.errors.children?.message ??
                      t("component.ingredients_empty")}
                  </p>
                </div>
              ) : (
                <div className="space-y-3">
                  {ingredientFields.map((field, index) => (
                    <IngredientRow
                      key={field.id}
                      index={index}
                      form={form}
                      onRemove={handleRemoveIngredient}
                    />
                  ))}
                </div>
              )}
            </SectionCard>

            {notesOpen ? (
              <SectionCard
                title={t("component.notes")}
                actions={
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => setNotesOpen(false)}
                    data-testid="notes-toggle-hide"
                  >
                    {t("component.notes_toggle_hide")}
                  </Button>
                }
              >
                <FormField
                  control={form.control}
                  name="notes"
                  render={({ field }) => (
                    <FormItem>
                      <FormControl>
                        <Textarea
                          placeholder={t("component.notes_placeholder")}
                          rows={4}
                          autoFocus
                          {...field}
                          value={field.value ?? ""}
                          onChange={(e) =>
                            field.onChange(e.target.value || null)
                          }
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </SectionCard>
            ) : (
              <button
                type="button"
                onClick={() => setNotesOpen(true)}
                className="flex w-full items-center justify-center gap-1.5 rounded-xl border border-dashed border-line/40 bg-panel/30 px-6 py-4 text-sm text-ink-muted transition-colors hover:border-primary/40 hover:text-ink"
                data-testid="notes-toggle-add"
              >
                <Plus className="size-4" aria-hidden />
                {t("component.notes_toggle_add")}
              </button>
            )}
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
                <p className="text-sm text-ink-muted">
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
                  {(variantsData.items as ComposedFood[]).map((variant) => (
                    <Link
                      key={variant.id}
                      to="/recipes/$id/edit"
                      params={{ id: String(variant.id) }}
                      className="flex w-40 shrink-0 flex-col overflow-hidden rounded-xl border border-line bg-panel transition-colors hover:border-ink-muted"
                      data-testid={`variant-card-${variant.id}`}
                    >
                      <div className="aspect-[4/3] overflow-hidden bg-line">
                        {variant.image_path ? (
                          <img
                            src={imageURL(
                              variant.image_path,
                              variant.updated_at
                            )}
                            alt=""
                            className="h-full w-full object-cover"
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
                        <p className="truncate text-sm font-medium text-ink">
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
              {isEdit && component ? (
                <Link to="/recipes/$id" params={{ id: String(component.id) }}>
                  {t("common.cancel")}
                </Link>
              ) : (
                <Link to="/recipes">{t("common.cancel")}</Link>
              )}
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
  ingredients: ComposedFoodFormValues["children"],
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

/** The recipe's own diet class, or null when it follows the ingredients. */
function dietOverride(
  food: ComposedFood
): ComposedFoodFormValues["diet_class"] {
  return food.diet_class_source === "user" && food.diet_class !== "unknown"
    ? food.diet_class
    : null
}

const FREEZABLE_OPTIONS = [
  { value: "unknown", stored: null, label: "component.freezable_unknown" },
  { value: "yes", stored: true, label: "component.freezable_yes" },
  { value: "no", stored: false, label: "component.freezable_no" },
] as const

/** Keep days (1–7, empty = the household default) and freezable (D15, F9).
 *  Above 4 days a food-safety note appears. */
function StorageFields() {
  const { t } = useTranslation()
  const form = useFormContext<ComposedFoodFormValues>()
  const defaultKeep = Number(
    useSettingField("plan.default_keep_days", "3").value
  )
  const keep = Number(useWatch({ control: form.control, name: "keep_days" }))

  return (
    <SectionCard title={t("component.storage")}>
      <div className="grid gap-4 sm:grid-cols-2">
        <FormField
          control={form.control}
          name="keep_days"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t("component.keep_days")}</FormLabel>
              <FormControl>
                <Input
                  type="number"
                  inputMode="numeric"
                  min={1}
                  max={7}
                  placeholder={t("component.keep_days_placeholder", {
                    count: defaultKeep,
                  })}
                  {...field}
                  value={field.value ?? ""}
                  data-testid="recipe-keep-days"
                />
              </FormControl>
              <FormDescription>{t("component.keep_days_hint")}</FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="freezable"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t("component.freezable")}</FormLabel>
              <Select
                value={
                  FREEZABLE_OPTIONS.find((o) => o.stored === field.value)
                    ?.value ?? "unknown"
                }
                onValueChange={(v) =>
                  field.onChange(
                    FREEZABLE_OPTIONS.find((o) => o.value === v)?.stored ?? null
                  )
                }
              >
                <FormControl>
                  <SelectTrigger data-testid="recipe-freezable">
                    <SelectValue />
                  </SelectTrigger>
                </FormControl>
                <SelectContent position="popper">
                  {FREEZABLE_OPTIONS.map((o) => (
                    <SelectItem key={o.value} value={o.value}>
                      {t(o.label)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>
      {keep > 4 && (
        <p
          role="status"
          className="flex items-start gap-2 text-small text-ink"
          data-testid="recipe-keep-days-warning"
        >
          <TriangleAlert
            className="mt-0.5 size-4 shrink-0 text-warning"
            strokeWidth={1.5}
            aria-hidden
          />
          {t("component.keep_days_warning")}
        </p>
      )}
    </SectionCard>
  )
}
