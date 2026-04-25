import { Link } from "@tanstack/react-router"
import { BookmarkPlus, Pencil, Sprout, Trash2 } from "lucide-react"
import { useState } from "react"
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
import { Skeleton } from "@/components/ui/skeleton"
import { useFoods } from "@/lib/queries/foods"
import {
  useDeleteTemplate,
  useTemplates,
  useUpdateTemplate,
} from "@/lib/queries/templates"

import { TemplateRenameDialog } from "./TemplateRenameDialog"

export function TemplateList() {
  const { t } = useTranslation()
  const { data: templates, isLoading } = useTemplates()
  const { data: componentsData } = useFoods({
    kind: "composed",
    limit: 200,
    offset: 0,
  })
  const componentsById = new Map(
    componentsData?.items.map((c) => [c.id, c] as const) ?? []
  )

  const [deleteId, setDeleteId] = useState<number | null>(null)
  const [renameId, setRenameId] = useState<number | null>(null)

  const deleteMutation = useDeleteTemplate()
  const renameMutation = useUpdateTemplate()

  function handleDelete() {
    if (deleteId === null) return
    deleteMutation.mutate(deleteId, {
      onSuccess: () => setDeleteId(null),
    })
  }

  const templateToRename = templates?.find((tpl) => tpl.id === renameId)

  return (
    <div className="mx-auto max-w-6xl space-y-10 px-4 py-8 md:px-8 md:py-12">
      <header
        className="flex flex-col items-start justify-between gap-6 md:flex-row md:items-end"
        data-testid="page-header"
      >
        <div className="max-w-2xl space-y-2">
          <span className="font-body text-xs font-bold tracking-widest text-primary uppercase">
            {t("template.eyebrow")}
          </span>
          <h1 className="font-heading text-4xl leading-tight font-extrabold tracking-tight text-on-surface md:text-5xl">
            {t("template.title")}
          </h1>
          <p className="max-w-lg text-base leading-relaxed text-on-surface-variant md:text-lg">
            {t("template.subtitle")}
          </p>
        </div>
        <Button asChild>
          <Link to="/templates/new">
            <BookmarkPlus className="size-4" />
            {t("template.new")}
          </Link>
        </Button>
      </header>

      {isLoading ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-44 w-full rounded-lg" />
          ))}
        </div>
      ) : !templates?.length ? (
        <EmptyState />
      ) : (
        <ul
          role="list"
          className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3"
          data-testid="template-grid"
        >
          {templates.map((tpl) => (
            <li
              key={tpl.id}
              className="group relative overflow-hidden rounded-lg border border-border bg-card transition-all hover:-translate-y-0.5 hover:border-primary/40 hover:shadow-sm"
              data-testid={`template-card-${tpl.id}`}
            >
              <span
                aria-hidden="true"
                className="absolute inset-x-0 top-0 h-1 bg-accent"
              />
              <div className="flex w-full flex-col items-start gap-3 px-5 pt-5 pb-4">
                <div className="flex w-full items-start justify-between gap-2">
                  <h2 className="text-lg leading-tight font-semibold tracking-tight">
                    {tpl.name}
                  </h2>
                  <Badge variant="secondary" className="shrink-0 text-xs">
                    {t("template.components_count", {
                      count: tpl.components.length,
                    })}
                  </Badge>
                </div>
                <div className="flex flex-wrap gap-1.5">
                  {tpl.components.length === 0 ? (
                    <span className="text-xs text-muted-foreground">
                      {t("template.no_components")}
                    </span>
                  ) : (
                    tpl.components.slice(0, 4).map((tc) => {
                      const c = componentsById.get(tc.food_id)
                      return (
                        <Badge
                          key={tc.id}
                          variant="outline"
                          className="text-xs font-normal"
                        >
                          {c?.name ?? `#${tc.food_id}`}
                        </Badge>
                      )
                    })
                  )}
                  {tpl.components.length > 4 && (
                    <Badge variant="outline" className="text-xs font-normal">
                      +{tpl.components.length - 4}
                    </Badge>
                  )}
                </div>
              </div>
              <div className="flex items-center justify-end gap-1 border-t border-dashed border-border/60 px-3 py-2">
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t("template.rename")}
                  onClick={() => setRenameId(tpl.id)}
                >
                  <Pencil className="size-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t("common.delete")}
                  onClick={() => setDeleteId(tpl.id)}
                >
                  <Trash2 className="size-3.5" />
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}

      <Dialog open={deleteId !== null} onOpenChange={() => setDeleteId(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("template.delete_confirm_title")}</DialogTitle>
            <DialogDescription>
              {t("template.delete_confirm_body")}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteId(null)}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={handleDelete}
              disabled={deleteMutation.isPending}
            >
              {t("common.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {templateToRename && (
        <TemplateRenameDialog
          open={renameId !== null}
          onOpenChange={(o) => !o && setRenameId(null)}
          defaultName={templateToRename.name}
          onSubmit={(name) =>
            renameMutation.mutate(
              { id: templateToRename.id, input: { name } },
              { onSuccess: () => setRenameId(null) }
            )
          }
          pending={renameMutation.isPending}
        />
      )}
    </div>
  )
}

function EmptyState() {
  const { t } = useTranslation()
  return (
    <div
      className="flex flex-col items-center gap-4 rounded-xl border border-dashed border-border bg-muted/40 px-8 py-16 text-center"
      data-testid="template-empty"
    >
      <div className="rounded-full border border-border/60 bg-background p-3">
        <Sprout className="size-6 text-primary" aria-hidden="true" />
      </div>
      <div className="space-y-1">
        <p className="text-base font-medium">{t("template.empty_title")}</p>
        <p className="max-w-sm text-sm text-muted-foreground">
          {t("template.empty_body")}
        </p>
      </div>
    </div>
  )
}
