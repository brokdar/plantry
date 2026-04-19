import { useState } from "react"
import { useForm, useWatch, type Resolver } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { useTranslation } from "react-i18next"
import { Link } from "@tanstack/react-router"
import { Loader2, Trash2 } from "lucide-react"

import { MacroBar } from "@/components/editorial/MacroBar"
import { SectionCard } from "@/components/editorial/SectionCard"
import { StickyActionBar } from "@/components/editorial/StickyActionBar"
import { ImageField } from "@/components/images/ImageField"
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

import { LookupPanel } from "./LookupPanel"
import { MacroFieldSet } from "./MacroFieldSet"
import { PortionsEditor } from "./PortionsEditor"

import {
  ingredientSchema,
  type IngredientFormValues,
} from "@/lib/schemas/ingredient"
import {
  useCreateIngredient,
  useUpdateIngredient,
  useDeleteIngredient,
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

function candidateToFormValues(c: LookupCandidate): IngredientFormValues {
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
        }
      : emptyValues(),
  })

  const createMutation = useCreateIngredient()
  const updateMutation = useUpdateIngredient()
  const deleteMutation = useDeleteIngredient()

  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const isPending = isEdit ? updateMutation.isPending : createMutation.isPending

  const values = useWatch({ control: form.control })
  const kcal = Number(values.kcal_100g) || 0
  const protein = Number(values.protein_100g) || 0
  const fat = Number(values.fat_100g) || 0
  const carbs = Number(values.carbs_100g) || 0
  const proteinKcal = protein * 4
  const fatKcal = fat * 9
  const carbsKcal = carbs * 4
  const totalKcal = Math.max(proteinKcal + fatKcal + carbsKcal, 1)

  async function onSubmit(v: IngredientFormValues) {
    try {
      if (isEdit && ingredient) {
        await updateMutation.mutateAsync({ id: ingredient.id, data: v })
      } else {
        await createMutation.mutateAsync(v)
      }
      onSuccess?.()
    } catch (err) {
      if (err instanceof ApiError) {
        form.setError("root", { message: t(err.messageKey) })
      }
    }
  }

  function handleLookupSelect(candidate: LookupCandidate) {
    form.reset(candidateToFormValues(candidate))
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
              <MacroBar
                thickness="md"
                track="surface-container-highest"
                segments={[
                  {
                    value: proteinKcal,
                    color: "primary",
                    label: t("ingredient.protein"),
                  },
                  {
                    value: carbsKcal,
                    color: "tertiary",
                    label: t("ingredient.carbs"),
                  },
                  {
                    value: fatKcal,
                    color: "secondary",
                    label: t("ingredient.fat"),
                  },
                ]}
                max={totalKcal}
              />
              <div className="flex flex-wrap gap-3 text-xs text-on-surface-variant">
                <LegendDot color="bg-primary" label={t("ingredient.protein")} />
                <LegendDot color="bg-tertiary" label={t("ingredient.carbs")} />
                <LegendDot color="bg-outline" label={t("ingredient.fat")} />
              </div>
            </SectionCard>

            {isEdit && ingredient && (
              <SectionCard title={t("portion.title")}>
                <PortionsEditor ingredientId={ingredient.id} />
              </SectionCard>
            )}
          </div>

          {/* Right column */}
          <div className="space-y-4 lg:sticky lg:top-6 lg:col-span-5 lg:self-start">
            {isEdit && ingredient && (
              <SectionCard
                title={t("image.section_title")}
                description={t("ingredient.section_image_hint")}
              >
                <ImageField
                  entityType="ingredients"
                  entityId={ingredient.id}
                  currentImagePath={ingredient.image_path}
                  onImageChange={() => {}}
                />
              </SectionCard>
            )}

            {!isEdit && (
              <SectionCard
                title={t("lookup.title")}
                description={t("ingredient.section_lookup_hint")}
              >
                <LookupPanel onSelect={handleLookupSelect} />
              </SectionCard>
            )}

            <SectionCard
              title={t("ingredient.per_100g")}
              testId="ingredient-nutrition-summary"
            >
              <p className="font-heading text-4xl font-extrabold text-on-surface">
                {Math.round(kcal)}
                <span className="ml-2 text-sm font-medium tracking-widest text-on-surface-variant uppercase">
                  kcal
                </span>
              </p>
              <dl className="grid grid-cols-3 gap-3">
                <NutrientStat
                  label={t("ingredient.protein")}
                  value={protein}
                  dot="bg-primary"
                />
                <NutrientStat
                  label={t("ingredient.carbs")}
                  value={carbs}
                  dot="bg-tertiary"
                />
                <NutrientStat
                  label={t("ingredient.fat")}
                  value={fat}
                  dot="bg-outline"
                />
              </dl>
            </SectionCard>
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

function NutrientStat({
  label,
  value,
  dot,
}: {
  label: string
  value: number
  dot: string
}) {
  return (
    <div className="rounded-xl bg-surface-container px-3 py-2">
      <p className="flex items-center gap-1.5 text-[10px] tracking-widest text-on-surface-variant uppercase">
        <span className={cn("inline-block size-1.5 rounded-full", dot)} />
        {label}
      </p>
      <p className="mt-0.5 font-heading text-lg font-bold text-on-surface">
        {value.toFixed(1)}
        <span className="ml-1 text-xs font-medium text-on-surface-variant">
          g
        </span>
      </p>
    </div>
  )
}

function LegendDot({ color, label }: { color: string; label: string }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span
        className={cn("inline-block size-2 rounded-full", color)}
        aria-hidden
      />
      {label}
    </span>
  )
}
