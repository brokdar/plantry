import { useState } from "react"
import { useForm, type Resolver } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { useTranslation } from "react-i18next"
import { Link } from "@tanstack/react-router"
import { ArrowLeft } from "lucide-react"
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { MacroFieldSet } from "./MacroFieldSet"
import { LookupPanel } from "./LookupPanel"
import { ImageField } from "@/components/images/ImageField"
import { PortionsEditor } from "./PortionsEditor"
import {
  ingredientSchema,
  type IngredientFormValues,
} from "@/lib/schemas/ingredient"
import {
  useCreateIngredient,
  useUpdateIngredient,
} from "@/lib/queries/ingredients"
import type { Ingredient } from "@/lib/api/ingredients"
import type { LookupCandidate } from "@/lib/api/lookup"
import { ApiError } from "@/lib/api/client"

interface IngredientEditorProps {
  ingredient?: Ingredient
  onSuccess?: () => void
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

function IngredientForm({
  form,
  isPending,
  onSubmit,
  children,
}: {
  form: ReturnType<typeof useForm<IngredientFormValues>>
  isPending: boolean
  onSubmit: (values: IngredientFormValues) => void
  children?: React.ReactNode
}) {
  const { t } = useTranslation()

  return (
    <Form {...form}>
      <form
        noValidate
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-6"
      >
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

        <div className="space-y-3">
          <h3 className="text-sm font-medium">{t("ingredient.macros")}</h3>
          <MacroFieldSet control={form.control} disabled={isPending} />
        </div>

        {form.formState.errors.root && (
          <p className="text-sm text-destructive">
            {form.formState.errors.root.message}
          </p>
        )}

        <div className="flex gap-3">
          <Button type="submit" disabled={isPending}>
            {t("common.save")}
          </Button>
          <Button variant="outline" asChild>
            <Link to="/ingredients">{t("common.cancel")}</Link>
          </Button>
        </div>

        {children}
      </form>
    </Form>
  )
}

export function IngredientEditor({
  ingredient,
  onSuccess,
}: IngredientEditorProps) {
  const { t } = useTranslation()
  const isEdit = !!ingredient
  const [selectedCandidate, setSelectedCandidate] =
    useState<LookupCandidate | null>(null)

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
      : {
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
        },
  })

  const createMutation = useCreateIngredient()
  const updateMutation = useUpdateIngredient()

  const isPending = isEdit ? updateMutation.isPending : createMutation.isPending

  async function onSubmit(values: IngredientFormValues) {
    try {
      if (isEdit && ingredient) {
        await updateMutation.mutateAsync({ id: ingredient.id, data: values })
      } else {
        await createMutation.mutateAsync(values)
      }
      onSuccess?.()
    } catch (err) {
      if (err instanceof ApiError) {
        form.setError("root", { message: t(err.messageKey) })
      }
    }
  }

  function handleCandidateSelect(candidate: LookupCandidate) {
    const values = candidateToFormValues(candidate)
    form.reset(values)
    setSelectedCandidate(candidate)
  }

  function handleBackToSearch() {
    setSelectedCandidate(null)
    form.reset({
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
    })
  }

  // Edit mode: show form directly with image upload and portions
  if (isEdit) {
    return (
      <IngredientForm form={form} isPending={isPending} onSubmit={onSubmit}>
        {ingredient && (
          <div className="space-y-6 border-t pt-6">
            <ImageField
              entityType="ingredients"
              entityId={ingredient.id}
              currentImagePath={ingredient.image_path}
              onImageChange={() => {}}
            />
            <PortionsEditor ingredientId={ingredient.id} />
          </div>
        )}
      </IngredientForm>
    )
  }

  // Create mode with selected candidate: show populated form
  if (selectedCandidate) {
    return (
      <div className="space-y-4">
        <button
          type="button"
          onClick={handleBackToSearch}
          className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="size-4" />
          {t("lookup.back_to_search")}
        </button>
        <IngredientForm form={form} isPending={isPending} onSubmit={onSubmit} />
      </div>
    )
  }

  // Create mode: show tabs for search vs manual
  return (
    <Tabs defaultValue="search">
      <TabsList>
        <TabsTrigger value="search">{t("lookup.tab_search")}</TabsTrigger>
        <TabsTrigger value="manual">{t("lookup.tab_manual")}</TabsTrigger>
      </TabsList>
      <TabsContent value="search" className="mt-4">
        <LookupPanel onSelect={handleCandidateSelect} />
      </TabsContent>
      <TabsContent value="manual" className="mt-4">
        <IngredientForm form={form} isPending={isPending} onSubmit={onSubmit} />
      </TabsContent>
    </Tabs>
  )
}
