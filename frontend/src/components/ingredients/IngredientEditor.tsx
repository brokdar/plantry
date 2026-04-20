import { useState } from "react"
import { useForm, useWatch, type Resolver } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { useTranslation } from "react-i18next"
import { Link } from "@tanstack/react-router"
import { Loader2, RefreshCw, Trash2 } from "lucide-react"

import { MacroDistributionBar, MacroTriad } from "@/components/editorial/macros"
import { NutritionPanel } from "@/components/editorial/NutritionPanel"
import { SectionCard } from "@/components/editorial/SectionCard"
import { StickyActionBar } from "@/components/editorial/StickyActionBar"
import { ImageField } from "@/components/images/ImageField"
import { useFetchImageFromUrl, useUploadImage } from "@/lib/queries/images"
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
import { cn } from "@/lib/utils"

import {
  EXTENDED_MACRO_FIELDS,
  ExtendedNutrientFieldSet,
  MINERAL_FIELDS,
  VITAMIN_FIELDS,
} from "./ExtendedNutrientFieldSet"
import { LookupPanel } from "./LookupPanel"
import { MacroFieldSet } from "./MacroFieldSet"
import { PortionsEditor } from "./PortionsEditor"

import { EXTENDED_NUTRIENT_KEYS } from "@/lib/api/ingredients"

import {
  ingredientSchema,
  type IngredientFormValues,
} from "@/lib/schemas/ingredient"
import {
  useCreateIngredient,
  useUpdateIngredient,
  useDeleteIngredient,
  useRefetchIngredient,
} from "@/lib/queries/ingredients"
import type { Ingredient } from "@/lib/api/ingredients"
import type { LookupCandidate } from "@/lib/api/lookup"
import { ApiError } from "@/lib/api/client"

interface IngredientEditorProps {
  ingredient?: Ingredient
  onSuccess?: () => void
  onDeleted?: () => void
}

const SOURCE_DOT: Record<string, string> = {
  fdc: "bg-primary",
  off: "bg-tertiary",
  manual: "bg-on-surface-variant/50",
}

function emptyExtendedNutrients() {
  return Object.fromEntries(
    EXTENDED_NUTRIENT_KEYS.map((k) => [k, null])
  ) as Record<(typeof EXTENDED_NUTRIENT_KEYS)[number], number | null>
}

function candidateToFormValues(c: LookupCandidate): IngredientFormValues {
  const extended = Object.fromEntries(
    EXTENDED_NUTRIENT_KEYS.map((k) => [k, c[k] ?? null])
  ) as Record<(typeof EXTENDED_NUTRIENT_KEYS)[number], number | null>
  return {
    name: c.name,
    source: c.source,
    barcode: c.barcode,
    off_id: c.source === "off" ? (c.barcode ?? null) : null,
    fdc_id: c.fdc_id ? String(c.fdc_id) : null,
    kcal_100g: c.kcal_100g ?? 0,
    protein_100g: c.protein_100g ?? 0,
    fat_100g: c.fat_100g ?? 0,
    carbs_100g: c.carbs_100g ?? 0,
    fiber_100g: c.fiber_100g ?? 0,
    sodium_100g: c.sodium_100g ?? 0,
    ...extended,
  }
}

function emptyValues(): IngredientFormValues {
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
    ...emptyExtendedNutrients(),
  }
}

export function IngredientEditor({
  ingredient,
  onSuccess,
  onDeleted,
}: IngredientEditorProps) {
  const { t } = useTranslation()
  const isEdit = !!ingredient

  const form = useForm<IngredientFormValues>({
    resolver: zodResolver(ingredientSchema) as Resolver<IngredientFormValues>,
    defaultValues: ingredient
      ? {
          name: ingredient.name,
          source: ingredient.source,
          barcode: ingredient.barcode,
          off_id: ingredient.off_id,
          fdc_id: ingredient.fdc_id,
          kcal_100g: ingredient.kcal_100g,
          protein_100g: ingredient.protein_100g,
          fat_100g: ingredient.fat_100g,
          carbs_100g: ingredient.carbs_100g,
          fiber_100g: ingredient.fiber_100g,
          sodium_100g: ingredient.sodium_100g,
          ...(Object.fromEntries(
            EXTENDED_NUTRIENT_KEYS.map((k) => [k, ingredient[k] ?? null])
          ) as Record<(typeof EXTENDED_NUTRIENT_KEYS)[number], number | null>),
        }
      : emptyValues(),
  })

  const createMutation = useCreateIngredient()
  const updateMutation = useUpdateIngredient()
  const deleteMutation = useDeleteIngredient()
  const refetchMutation = useRefetchIngredient()
  const uploadMutation = useUploadImage()
  const fetchImageMutation = useFetchImageFromUrl()

  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [refetchError, setRefetchError] = useState<string | null>(null)
  const [stagedImage, setStagedImage] = useState<Blob | null>(null)

  const canRefetch =
    !!ingredient &&
    ((ingredient.barcode && ingredient.barcode.length > 0) ||
      (ingredient.fdc_id && ingredient.fdc_id.length > 0))

  const isPending = isEdit ? updateMutation.isPending : createMutation.isPending

  const values = useWatch({ control: form.control })
  const kcal = Number(values.kcal_100g) || 0
  const protein = Number(values.protein_100g) || 0
  const fat = Number(values.fat_100g) || 0
  const carbs = Number(values.carbs_100g) || 0
  const fiber = Number(values.fiber_100g) || 0
  const sodium = Number(values.sodium_100g) || 0

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
      }
      onSuccess?.()
    } catch (err) {
      if (err instanceof ApiError) {
        form.setError("root", { message: t(err.messageKey) })
      }
    }
  }

  async function handleLookupSelect(candidate: LookupCandidate) {
    form.reset(candidateToFormValues(candidate))
    setStagedImage(null)
    if (candidate.image_url) {
      try {
        const blob = await fetchImageMutation.mutateAsync({
          url: candidate.image_url,
        })
        setStagedImage(blob)
      } catch {
        // Image is nice-to-have; silently skip so the rest of the lookup
        // payload still applies.
      }
    }
  }

  async function handleRefetch() {
    if (!ingredient || !canRefetch) return
    setRefetchError(null)
    try {
      const updated = await refetchMutation.mutateAsync({
        id: ingredient.id,
        lang: undefined,
      })
      // Replace the form values with the freshly-fetched nutrient data so the
      // user sees the update without navigating away.
      form.reset({
        name: updated.name,
        source: updated.source,
        barcode: updated.barcode,
        off_id: updated.off_id,
        fdc_id: updated.fdc_id,
        kcal_100g: updated.kcal_100g,
        protein_100g: updated.protein_100g,
        fat_100g: updated.fat_100g,
        carbs_100g: updated.carbs_100g,
        fiber_100g: updated.fiber_100g,
        sodium_100g: updated.sodium_100g,
        ...(Object.fromEntries(
          EXTENDED_NUTRIENT_KEYS.map((k) => [k, updated[k] ?? null])
        ) as Record<(typeof EXTENDED_NUTRIENT_KEYS)[number], number | null>),
      })
    } catch (err) {
      if (err instanceof ApiError) {
        setRefetchError(t(err.messageKey))
      } else {
        setRefetchError(t("error.server"))
      }
    }
  }

  function confirmDelete() {
    if (!ingredient) return
    setDeleteError(null)
    deleteMutation.mutate(ingredient.id, {
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

  const source = ingredient?.source ?? "manual"

  return (
    <Form {...form}>
      <form
        noValidate
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-6"
      >
        {form.formState.errors.root && (
          <p className="text-sm text-destructive">
            {form.formState.errors.root.message}
          </p>
        )}

        {isEdit && ingredient && (
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="secondary" className="gap-1.5">
              <span
                className={cn(
                  "inline-block size-1.5 rounded-full",
                  SOURCE_DOT[source] ?? "bg-on-surface-variant/50"
                )}
                aria-hidden
              />
              {t(`ingredient.source_${source}`, { defaultValue: source })}
            </Badge>
            {canRefetch && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-7 gap-1.5 text-xs"
                onClick={handleRefetch}
                disabled={refetchMutation.isPending || isPending}
                data-testid="ingredient-refetch"
              >
                {refetchMutation.isPending ? (
                  <Loader2 className="size-3.5 animate-spin" aria-hidden />
                ) : (
                  <RefreshCw className="size-3.5" aria-hidden />
                )}
                {t("ingredient.refetch")}
              </Button>
            )}
            {refetchError && (
              <span className="text-xs text-destructive">{refetchError}</span>
            )}
          </div>
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

            {isEdit && ingredient && (
              <SectionCard title={t("portion.title")}>
                <PortionsEditor ingredientId={ingredient.id} />
              </SectionCard>
            )}
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

            {!isEdit && (
              <SectionCard
                title={t("lookup.title")}
                description={t("ingredient.section_lookup_hint")}
              >
                <LookupPanel onSelect={handleLookupSelect} />
              </SectionCard>
            )}

            <NutritionPanel
              variant="static"
              title={t("ingredient.per_100g")}
              testId="ingredient-nutrition-summary"
              macros={{ kcal, protein, fat, carbs, fiber, sodium }}
            />
          </div>
        </div>

        <StickyActionBar
          primary={
            <Button
              type="submit"
              disabled={isPending || !form.watch("name").trim()}
            >
              {isPending && <Loader2 className="mr-2 size-4 animate-spin" />}
              {t("common.save")}
            </Button>
          }
          secondary={
            <Button variant="outline" asChild>
              <Link to="/ingredients">{t("common.cancel")}</Link>
            </Button>
          }
          destructive={
            isEdit ? (
              <Button
                type="button"
                variant="ghost"
                onClick={() => setDeleteOpen(true)}
                className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                data-testid="ingredient-delete"
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
            <DialogTitle>{t("ingredient.delete_confirm_title")}</DialogTitle>
            <DialogDescription>
              {t("ingredient.delete_confirm_body")}
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
