import { useEffect } from "react"
import { useForm } from "react-hook-form"
import { useTranslation } from "react-i18next"
import { Loader2 } from "lucide-react"

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
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import type { Food } from "@/lib/api/foods"
import { useCreateFood } from "@/lib/queries/foods"
import { ApiError } from "@/lib/api/client"

type FormValues = {
  name: string
  kcal_100g: string
  protein_100g: string
  carbs_100g: string
  fat_100g: string
}

type QuickCreateIngredientDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialName?: string
  onCreated: (ingredient: Food) => void
}

const EMPTY: FormValues = {
  name: "",
  kcal_100g: "",
  protein_100g: "",
  carbs_100g: "",
  fat_100g: "",
}

function toNumber(v: string): number {
  const n = Number(v)
  return Number.isFinite(n) && n >= 0 ? n : 0
}

export function QuickCreateIngredientDialog({
  open,
  onOpenChange,
  initialName = "",
  onCreated,
}: QuickCreateIngredientDialogProps) {
  const { t } = useTranslation()
  const create = useCreateFood()

  const form = useForm<FormValues>({ defaultValues: EMPTY })

  useEffect(() => {
    if (open) {
      form.reset({ ...EMPTY, name: initialName })
    }
    // Intentional: only reset on open transition / initialName change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, initialName])

  async function onSubmit(values: FormValues) {
    const name = values.name.trim()
    if (!name) {
      form.setError("name", { message: t("validation.required") })
      return
    }
    try {
      const ingredient = await create.mutateAsync({
        kind: "leaf",
        name,
        kcal_100g: toNumber(values.kcal_100g),
        protein_100g: toNumber(values.protein_100g),
        carbs_100g: toNumber(values.carbs_100g),
        fat_100g: toNumber(values.fat_100g),
      })
      onCreated(ingredient)
      onOpenChange(false)
    } catch (err) {
      const message =
        err instanceof ApiError ? t(err.messageKey) : t("error.server")
      form.setError("root", { message })
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("ingredient.quick_create.title")}</DialogTitle>
          <DialogDescription>
            {t("ingredient.quick_create.description")}
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit)}
            className="space-y-4"
            data-testid="quick-create-ingredient-form"
          >
            {form.formState.errors.root && (
              <p className="text-sm text-destructive">
                {form.formState.errors.root.message}
              </p>
            )}
            <FormField
              control={form.control}
              name="name"
              rules={{
                validate: (v) =>
                  v.trim().length > 0 || t("validation.required"),
              }}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("ingredient.name")}</FormLabel>
                  <FormControl>
                    <Input
                      autoFocus
                      placeholder={t("ingredient.name_placeholder")}
                      data-testid="quick-create-ingredient-name"
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className="grid grid-cols-2 gap-3">
              <FormField
                control={form.control}
                name="kcal_100g"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="text-xs">
                      {t("ingredient.kcal")}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        step="any"
                        min="0"
                        inputMode="decimal"
                        placeholder="0"
                        className="tabular-nums"
                        {...field}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="protein_100g"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="text-xs">
                      {t("ingredient.protein")}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        step="any"
                        min="0"
                        inputMode="decimal"
                        placeholder="0"
                        className="tabular-nums"
                        {...field}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="carbs_100g"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="text-xs">
                      {t("ingredient.carbs")}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        step="any"
                        min="0"
                        inputMode="decimal"
                        placeholder="0"
                        className="tabular-nums"
                        {...field}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="fat_100g"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="text-xs">
                      {t("ingredient.fat")}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        step="any"
                        min="0"
                        inputMode="decimal"
                        placeholder="0"
                        className="tabular-nums"
                        {...field}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            </div>
            <p className="text-xs text-on-surface-variant">
              {t("ingredient.quick_create.macros_hint")}
            </p>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
                disabled={create.isPending}
              >
                {t("common.cancel")}
              </Button>
              <Button
                type="submit"
                disabled={create.isPending}
                data-testid="quick-create-ingredient-save"
              >
                {create.isPending && (
                  <Loader2 className="mr-2 size-4 animate-spin" />
                )}
                {t("ingredient.quick_create.save")}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
