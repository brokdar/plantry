import { zodResolver } from "@hookform/resolvers/zod"
import { BookmarkPlus } from "lucide-react"
import { useForm } from "react-hook-form"
import { useTranslation } from "react-i18next"

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
import { useCreateTemplate } from "@/lib/queries/templates"
import { templateSchema, type TemplateFormValues } from "@/lib/schemas/template"

interface SaveAsTemplateDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  plateId: number | null
}

export function SaveAsTemplateDialog({
  open,
  onOpenChange,
  plateId,
}: SaveAsTemplateDialogProps) {
  const { t } = useTranslation()
  const createMutation = useCreateTemplate()
  const form = useForm<TemplateFormValues>({
    resolver: zodResolver(templateSchema),
    defaultValues: { name: "" },
  })

  function handleOpenChange(next: boolean) {
    if (!next) form.reset({ name: "" })
    onOpenChange(next)
  }

  function onSubmit(values: TemplateFormValues) {
    if (plateId === null) return
    createMutation.mutate(
      { name: values.name.trim(), from_plate_id: plateId },
      {
        onSuccess: () => {
          form.reset({ name: "" })
          onOpenChange(false)
        },
      }
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("template.save_as")}</DialogTitle>
          <DialogDescription>{t("template.save_as_body")}</DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
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
                  <FormMessage />
                </FormItem>
              )}
            />
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => handleOpenChange(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={createMutation.isPending}>
                <BookmarkPlus className="size-4" />
                {t("template.create")}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
