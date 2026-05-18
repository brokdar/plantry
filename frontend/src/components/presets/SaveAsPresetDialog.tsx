import { zodResolver } from "@hookform/resolvers/zod"
import { Bookmark, BookmarkPlus, X } from "lucide-react"
import { useEffect, useState } from "react"
import { useForm } from "react-hook-form"
import { useTranslation } from "react-i18next"

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
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { useCreatePreset, useKnownTags } from "@/lib/queries/presets"
import {
  savePresetFormSchema,
  type SavePresetFormValues,
} from "@/lib/schemas/preset"
import { toast, toastError } from "@/lib/toast"

export interface SaveAsPresetTarget {
  /** Plate IDs to materialise into the preset's plates list. */
  plateIds: number[]
  /** Pre-computed suggestion (e.g. dominant food name or "Bundle of 3"). */
  defaultName?: string
}

interface SaveAsPresetDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  target: SaveAsPresetTarget | null
}

export function SaveAsPresetDialog({
  open,
  onOpenChange,
  target,
}: SaveAsPresetDialogProps) {
  const { t } = useTranslation()
  const createMutation = useCreatePreset()
  const knownTagsQuery = useKnownTags()
  const isPending = createMutation.isPending

  const form = useForm<SavePresetFormValues>({
    resolver: zodResolver(savePresetFormSchema),
    defaultValues: { name: target?.defaultName ?? "", tags: [] },
  })
  const [tagInput, setTagInput] = useState("")

  useEffect(() => {
    if (open) {
      form.reset({ name: target?.defaultName ?? "", tags: [] })
      setTagInput("")
    }
  }, [open, target?.defaultName, form])

  const currentTags = form.watch("tags")

  function addTag(raw: string) {
    const clean = raw.trim().toLowerCase()
    if (!clean) return
    if (currentTags.includes(clean)) return
    form.setValue("tags", [...currentTags, clean], { shouldDirty: true })
    setTagInput("")
  }

  function removeTag(tag: string) {
    form.setValue(
      "tags",
      currentTags.filter((t) => t !== tag),
      { shouldDirty: true }
    )
  }

  function handleOpenChange(next: boolean) {
    if (!next) {
      form.reset({ name: "", tags: [] })
      setTagInput("")
    }
    onOpenChange(next)
  }

  function onSubmit(values: SavePresetFormValues) {
    if (!target) return
    const name = values.name.trim()
    createMutation.mutate(
      { name, plate_ids: target.plateIds, tags: values.tags },
      {
        onSuccess: () => {
          toast.success(t("preset.created", { name }))
          handleOpenChange(false)
        },
        onError: (err) => toastError(err, t),
      }
    )
  }

  const isBundle = (target?.plateIds.length ?? 0) > 1
  const titleKey = isBundle
    ? "preset.save_selection_as"
    : "preset.save_plate_as"
  const bodyKey = isBundle ? "preset.save_selection_body" : "preset.save_body"

  const tagSuggestions = (knownTagsQuery.data?.items ?? [])
    .filter((kt) => !currentTags.includes(kt.tag))
    .slice(0, 6)

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent data-testid="save-preset-dialog">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Bookmark className="size-4" aria-hidden />
            {t(titleKey)}
          </DialogTitle>
          <DialogDescription>{t(bodyKey)}</DialogDescription>
        </DialogHeader>

        {target && (
          <p
            className="rounded-md border border-outline-variant/40 bg-surface-container-low/40 px-3 py-2 font-mono text-[12px] text-on-surface-variant"
            data-testid="save-preset-preview"
          >
            {target.plateIds.length === 1
              ? t("preset.plates_count", { count: 1 })
              : t("preset.plates_count", { count: target.plateIds.length })}
          </p>
        )}

        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-5">
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("preset.name")}</FormLabel>
                  <FormControl>
                    <Input
                      autoFocus
                      placeholder={t("preset.name_placeholder")}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormItem>
              <FormLabel>{t("preset.tags_label")}</FormLabel>
              <FormControl>
                <div className="space-y-2">
                  {currentTags.length > 0 && (
                    <ul
                      role="list"
                      className="flex flex-wrap gap-1.5"
                      data-testid="save-preset-tags"
                    >
                      {currentTags.map((tag) => (
                        <li key={tag}>
                          <Badge
                            variant="secondary"
                            className="gap-1 pr-1 text-xs"
                          >
                            {tag}
                            <button
                              type="button"
                              onClick={() => removeTag(tag)}
                              className="rounded-sm p-0.5 text-muted-foreground hover:text-on-surface"
                              aria-label={`Remove tag ${tag}`}
                            >
                              <X className="size-3" />
                            </button>
                          </Badge>
                        </li>
                      ))}
                    </ul>
                  )}
                  <Input
                    value={tagInput}
                    onChange={(e) => setTagInput(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" || e.key === ",") {
                        e.preventDefault()
                        addTag(tagInput)
                      } else if (
                        e.key === "Backspace" &&
                        tagInput === "" &&
                        currentTags.length > 0
                      ) {
                        removeTag(currentTags[currentTags.length - 1]!)
                      }
                    }}
                    placeholder={t("preset.tags_placeholder")}
                    data-testid="save-preset-tag-input"
                  />
                </div>
              </FormControl>
              {tagSuggestions.length > 0 && (
                <ul
                  role="list"
                  className="flex flex-wrap gap-1.5"
                  aria-label="Tag suggestions"
                >
                  {tagSuggestions.map((kt) => (
                    <li key={kt.tag}>
                      <button
                        type="button"
                        onClick={() => addTag(kt.tag)}
                        className="rounded-full border border-dashed border-border/60 px-2 py-0.5 text-[11px] text-muted-foreground hover:border-primary/40 hover:text-on-surface"
                      >
                        + {kt.tag}
                      </button>
                    </li>
                  ))}
                </ul>
              )}
              <p className="text-[11px] text-muted-foreground">
                {t("preset.tags_help")}
              </p>
            </FormItem>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => handleOpenChange(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={isPending || !target}>
                <BookmarkPlus className="size-4" />
                {t("preset.create")}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
