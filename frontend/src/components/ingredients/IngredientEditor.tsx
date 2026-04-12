import { useForm, type Resolver } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { useTranslation } from "react-i18next"
import { Link } from "@tanstack/react-router"
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
import { MacroFieldSet } from "./MacroFieldSet"
import {
  ingredientSchema,
  type IngredientFormValues,
} from "@/lib/schemas/ingredient"
import {
  useCreateIngredient,
  useUpdateIngredient,
} from "@/lib/queries/ingredients"
import type { Ingredient } from "@/lib/api/ingredients"
import { ApiError } from "@/lib/api/client"

interface IngredientEditorProps {
  ingredient?: Ingredient
  onSuccess?: () => void
}

export function IngredientEditor({
  ingredient,
  onSuccess,
}: IngredientEditorProps) {
  const { t } = useTranslation()
  const isEdit = !!ingredient

  const form = useForm<IngredientFormValues>({
    resolver: zodResolver(ingredientSchema) as Resolver<IngredientFormValues>,
    defaultValues: ingredient
      ? {
          name: ingredient.name,
          kcal_100g: ingredient.kcal_100g,
          protein_100g: ingredient.protein_100g,
          fat_100g: ingredient.fat_100g,
          carbs_100g: ingredient.carbs_100g,
          fiber_100g: ingredient.fiber_100g,
          sodium_100g: ingredient.sodium_100g,
        }
      : {
          name: "",
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

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6">
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
      </form>
    </Form>
  )
}
