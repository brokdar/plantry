import { zodResolver } from "@hookform/resolvers/zod"
import {
  BookmarkPlus,
  CalendarDays,
  CalendarRange,
  Utensils,
} from "lucide-react"
import { useEffect } from "react"
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
import type { TemplateScope } from "@/lib/api/templates"
import {
  useCreateTemplate,
  useCreateTemplateFromRange,
} from "@/lib/queries/templates"
import { templateSchema, type TemplateFormValues } from "@/lib/schemas/template"
import { toast, toastError } from "@/lib/toast"

export type SaveAsTemplateTarget =
  | { scope: "slot"; plateId: number; componentCount: number }
  | { scope: "day"; date: string; plateCount: number }
  | { scope: "week"; from: string; to: string; plateCount: number }

interface SaveAsTemplateDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  target: SaveAsTemplateTarget | null
  /** Pre-computed name suggestion. The dialog seeds the input with it; the
   * user can overwrite freely. */
  defaultName?: string
}

const ICONS = {
  slot: Utensils,
  day: CalendarDays,
  week: CalendarRange,
} as const

export function SaveAsTemplateDialog({
  open,
  onOpenChange,
  target,
  defaultName,
}: SaveAsTemplateDialogProps) {
  const { t } = useTranslation()
  const createMutation = useCreateTemplate()
  const createFromRangeMutation = useCreateTemplateFromRange()
  const isPending =
    createMutation.isPending || createFromRangeMutation.isPending

  const form = useForm<TemplateFormValues>({
    resolver: zodResolver(templateSchema),
    defaultValues: { name: defaultName ?? "" },
  })

  // Seed the field whenever the dialog opens for a new target — react-hook-form
  // doesn't re-pick defaultValues on prop change, so reset() is required.
  useEffect(() => {
    if (open) form.reset({ name: defaultName ?? "" })
  }, [open, defaultName, form])

  const scope: TemplateScope | null = target?.scope ?? null
  const Icon = scope ? ICONS[scope] : BookmarkPlus
  const titleKey = scope ? `template.save_${scope}` : "template.save_as"
  const bodyKey = scope
    ? `template.save_${scope}_body`
    : "template.save_as_body"

  const previewLabel = (() => {
    if (!target) return null
    if (target.scope === "slot") {
      return t("template.save_slot_preview", {
        count: target.componentCount,
        defaultValue: `1 plate · ${target.componentCount} components`,
      })
    }
    if (target.scope === "day") {
      return target.plateCount === 0
        ? t("template.save_empty_day")
        : t("template.save_day_preview", { count: target.plateCount })
    }
    return target.plateCount === 0
      ? t("template.save_empty_week")
      : t("template.save_week_preview", { count: target.plateCount })
  })()

  const isEmpty =
    target !== null && target.scope !== "slot" && target.plateCount === 0

  function handleOpenChange(next: boolean) {
    if (!next) form.reset({ name: "" })
    onOpenChange(next)
  }

  function onSubmit(values: TemplateFormValues) {
    if (!target || isEmpty) return
    const name = values.name.trim()
    const handlers = {
      onSuccess: () => {
        toast.success(t("template.created", { name }))
        form.reset({ name: "" })
        onOpenChange(false)
      },
      onError: (err: unknown) => toastError(err, t),
    }
    if (target.scope === "slot") {
      createMutation.mutate(
        { name, scope: "slot", from_plate_id: target.plateId },
        handlers
      )
    } else if (target.scope === "day") {
      // Single-day range — backend auto-detects day scope from the range.
      createFromRangeMutation.mutate(
        { name, from: target.date, to: target.date },
        handlers
      )
    } else {
      createFromRangeMutation.mutate(
        { name, from: target.from, to: target.to },
        handlers
      )
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Icon className="size-4" aria-hidden />
            {t(titleKey)}
          </DialogTitle>
          <DialogDescription>{t(bodyKey)}</DialogDescription>
        </DialogHeader>
        {previewLabel && (
          <p
            className="rounded-md border border-outline-variant/40 bg-surface-container-low/40 px-3 py-2 font-mono text-[12px] text-on-surface-variant"
            data-testid="save-template-preview"
          >
            {previewLabel}
          </p>
        )}
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
              <Button type="submit" disabled={isPending || isEmpty}>
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
