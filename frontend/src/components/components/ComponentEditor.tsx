import { useState, useEffect } from "react"
import { useForm, useFieldArray } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { useTranslation } from "react-i18next"
import { Plus, Trash2, X } from "lucide-react"
import {
  Form,
  FormField,
  FormItem,
  FormLabel,
  FormControl,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { Badge } from "@/components/ui/badge"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { NutritionPreview } from "./NutritionPreview"
import {
  componentSchema,
  COMPONENT_ROLES,
  type ComponentFormValues,
} from "@/lib/schemas/component"
import {
  useCreateComponent,
  useUpdateComponent,
} from "@/lib/queries/components"
import { useIngredients, useIngredient } from "@/lib/queries/ingredients"
import { usePortions } from "@/lib/queries/portions"
import type { Component } from "@/lib/api/components"
import { ApiError } from "@/lib/api/client"

interface ComponentEditorProps {
  component?: Component
  onSuccess?: () => void
}

export function ComponentEditor({
  component,
  onSuccess,
}: ComponentEditorProps) {
  const { t } = useTranslation()
  const isEdit = !!component

  const form = useForm<ComponentFormValues>({
    resolver: zodResolver(componentSchema),
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
            ingredient_name: "",
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

  const watchedIngredients = form.watch("ingredients")
  const watchedPortions = form.watch("reference_portions")

  async function onSubmit(values: ComponentFormValues) {
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
      if (isEdit) {
        await updateMutation.mutateAsync({ id: component.id, data: input })
      } else {
        await createMutation.mutateAsync(input)
      }
      onSuccess?.()
    } catch (err) {
      if (err instanceof ApiError) {
        form.setError("root", { message: t(err.messageKey) })
      }
    }
  }

  // Tag management
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

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-8">
        {form.formState.errors.root && (
          <p className="text-sm text-destructive">
            {form.formState.errors.root.message}
          </p>
        )}

        {/* Basic fields */}
        <div className="grid gap-4 sm:grid-cols-2">
          <FormField
            control={form.control}
            name="name"
            render={({ field }) => (
              <FormItem>
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
        </div>

        <div className="grid gap-4 sm:grid-cols-3">
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

        <FormField
          control={form.control}
          name="notes"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t("component.notes")}</FormLabel>
              <FormControl>
                <Textarea
                  placeholder={t("component.notes_placeholder")}
                  {...field}
                  value={field.value ?? ""}
                  onChange={(e) => field.onChange(e.target.value || null)}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* Ingredients */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium">
              {t("component.ingredients")}
            </h3>
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
            >
              <Plus className="mr-1 size-4" />
              {t("component.add_ingredient")}
            </Button>
          </div>
          {ingredientFields.map((field, index) => (
            <IngredientRow
              key={field.id}
              index={index}
              form={form}
              onRemove={() => removeIngredient(index)}
            />
          ))}
        </div>

        {/* Instructions */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium">
              {t("component.instructions")}
            </h3>
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
            >
              <Plus className="mr-1 size-4" />
              {t("component.add_instruction")}
            </Button>
          </div>
          {instructionFields.map((field, index) => (
            <div key={field.id} className="flex items-start gap-2">
              <span className="mt-2 text-sm font-medium text-muted-foreground">
                {index + 1}.
              </span>
              <FormField
                control={form.control}
                name={`instructions.${index}.text`}
                render={({ field }) => (
                  <FormItem className="flex-1">
                    <FormControl>
                      <Input
                        placeholder={t("component.instruction_placeholder")}
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
                onClick={() => removeInstruction(index)}
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          ))}
        </div>

        {/* Tags */}
        <div className="space-y-3">
          <h3 className="text-sm font-medium">{t("component.tags")}</h3>
          <div className="flex flex-wrap gap-2">
            {watchedTags.map((tag) => (
              <Badge key={tag} variant="secondary" className="gap-1">
                {tag}
                <button
                  type="button"
                  onClick={() => removeTag(tag)}
                  className="ml-1 rounded-full hover:bg-muted"
                >
                  <X className="size-3" />
                </button>
              </Badge>
            ))}
          </div>
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
            <Button type="button" variant="outline" size="sm" onClick={addTag}>
              {t("component.add_tag")}
            </Button>
          </div>
        </div>

        {/* Nutrition Preview */}
        {watchedIngredients.length > 0 && (
          <NutritionPreview
            ingredients={watchedIngredients}
            referencePortions={watchedPortions}
          />
        )}

        {/* Actions */}
        <div className="flex gap-3">
          <Button
            type="submit"
            disabled={createMutation.isPending || updateMutation.isPending}
          >
            {t("common.save")}
          </Button>
        </div>
      </form>
    </Form>
  )
}

// --- Ingredient row sub-component ---

function IngredientRow({
  index,
  form,
  onRemove,
}: {
  index: number
  form: ReturnType<typeof useForm<ComponentFormValues>>
  onRemove: () => void
}) {
  const { t } = useTranslation()
  const [ingredientSearch, setIngredientSearch] = useState("")
  const { data: ingredientResults } = useIngredients({
    search: ingredientSearch || undefined,
    limit: 10,
  })

  const ingredientId = form.watch(`ingredients.${index}.ingredient_id`)
  const unit = form.watch(`ingredients.${index}.unit`)

  const { data: portions } = usePortions(ingredientId)
  // Fetch full ingredient to populate per-100g macros in edit mode, where
  // the component API returns only the join-table fields (no macros).
  const { data: ingredientDetail } = useIngredient(ingredientId)
  useEffect(() => {
    if (!ingredientDetail) return
    // Only backfill when macros were not already set (e.g. via selectIngredient).
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
  }, [ingredientDetail]) // eslint-disable-line react-hooks/exhaustive-deps

  function selectIngredient(ing: {
    id: number
    name: string
    kcal_100g: number
    protein_100g: number
    fat_100g: number
    carbs_100g: number
    fiber_100g: number
    sodium_100g: number
  }) {
    form.setValue(`ingredients.${index}.ingredient_id`, ing.id)
    form.setValue(`ingredients.${index}.ingredient_name`, ing.name)
    form.setValue(`ingredients.${index}.kcal_100g`, ing.kcal_100g)
    form.setValue(`ingredients.${index}.protein_100g`, ing.protein_100g)
    form.setValue(`ingredients.${index}.fat_100g`, ing.fat_100g)
    form.setValue(`ingredients.${index}.carbs_100g`, ing.carbs_100g)
    form.setValue(`ingredients.${index}.fiber_100g`, ing.fiber_100g)
    form.setValue(`ingredients.${index}.sodium_100g`, ing.sodium_100g)
    setIngredientSearch("")
  }

  function handleUnitChange(newUnit: string) {
    form.setValue(`ingredients.${index}.unit`, newUnit)
    recalcGrams(form.getValues(`ingredients.${index}.amount`), newUnit)
  }

  function handleAmountChange(newAmount: number) {
    recalcGrams(newAmount, unit)
  }

  function recalcGrams(amount: number, u: string) {
    if (u === "g" || u === "ml") {
      form.setValue(`ingredients.${index}.grams`, amount)
    } else if (portions) {
      const match = portions.find((p) => p.unit === u)
      if (match) {
        form.setValue(`ingredients.${index}.grams`, amount * match.grams)
      }
    }
  }

  const ingredientName = form.watch(`ingredients.${index}.ingredient_name`)

  return (
    <div className="space-y-2 rounded-md border border-border p-3">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">
          {ingredientId > 0
            ? ingredientName || `#${ingredientId}`
            : t("component.select_ingredient")}
        </span>
        <Button type="button" variant="ghost" size="icon-sm" onClick={onRemove}>
          <Trash2 className="size-4" />
        </Button>
      </div>

      {ingredientId === 0 && (
        <div className="relative">
          <Input
            placeholder={t("ingredient.search_placeholder")}
            value={ingredientSearch}
            onChange={(e) => setIngredientSearch(e.target.value)}
          />
          {ingredientSearch && ingredientResults?.items.length ? (
            <div className="absolute z-10 mt-1 w-full rounded-md border border-border bg-popover shadow-md">
              {ingredientResults.items.map((ing) => (
                <button
                  key={ing.id}
                  type="button"
                  className="w-full px-3 py-2 text-left text-sm hover:bg-accent"
                  onClick={() => selectIngredient(ing)}
                >
                  {ing.name}
                </button>
              ))}
            </div>
          ) : null}
        </div>
      )}

      {ingredientId > 0 && (
        <div className="grid grid-cols-3 gap-2">
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
                    {...field}
                    onChange={(e) => {
                      field.onChange(e)
                      handleAmountChange(Number(e.target.value))
                    }}
                  />
                </FormControl>
              </FormItem>
            )}
          />
          <FormItem>
            <FormLabel className="text-xs">{t("component.unit")}</FormLabel>
            <Select value={unit} onValueChange={handleUnitChange}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="g">g</SelectItem>
                <SelectItem value="ml">ml</SelectItem>
                {portions?.map((p) => (
                  <SelectItem key={p.unit} value={p.unit}>
                    {p.unit}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </FormItem>
          <FormItem>
            <FormLabel className="text-xs">{t("component.grams")}</FormLabel>
            <Input
              type="number"
              value={form.watch(`ingredients.${index}.grams`).toFixed(1)}
              disabled
              className="bg-muted"
            />
          </FormItem>
        </div>
      )}
    </div>
  )
}
