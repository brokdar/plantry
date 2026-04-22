import { zodResolver } from "@hookform/resolvers/zod"
import { useNavigate } from "@tanstack/react-router"
import { BookmarkPlus } from "lucide-react"
import { useForm } from "react-hook-form"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { useCreateTemplate } from "@/lib/queries/templates"
import { templateSchema, type TemplateFormValues } from "@/lib/schemas/template"

export function TemplateForm() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const createMutation = useCreateTemplate()

  const form = useForm<TemplateFormValues>({
    resolver: zodResolver(templateSchema),
    defaultValues: { name: "" },
  })

  function onSubmit(values: TemplateFormValues) {
    createMutation.mutate(
      { name: values.name.trim() },
      {
        onSuccess: () => navigate({ to: "/templates" }),
      }
    )
  }

  return (
    <div className="mx-auto max-w-md space-y-8">
      <header className="space-y-2">
        <p className="text-xs font-medium tracking-[0.2em] text-muted-foreground uppercase">
          {t("template.eyebrow")}
        </p>
        <h1 className="text-3xl font-semibold tracking-tight">
          {t("template.new")}
        </h1>
        <p className="text-sm text-muted-foreground">
          {t("template.new_body")}
        </p>
      </header>

      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6">
          <FormField
            control={form.control}
            name="name"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t("template.name")}</FormLabel>
                <FormControl>
                  <Input
                    autoFocus
                    placeholder={t("template.name_placeholder")}
                    {...field}
                  />
                </FormControl>
                <FormDescription>{t("template.name_help")}</FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className="flex items-center justify-end gap-2 border-t border-dashed border-border/60 pt-4">
            <Button
              type="button"
              variant="outline"
              onClick={() => navigate({ to: "/templates" })}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={createMutation.isPending}>
              <BookmarkPlus className="size-4" />
              {t("template.create")}
            </Button>
          </div>
        </form>
      </Form>
    </div>
  )
}
