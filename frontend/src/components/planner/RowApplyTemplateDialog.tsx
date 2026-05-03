import { Bookmark } from "lucide-react"
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
import { applyTemplate, type Template } from "@/lib/api/templates"
import { plateKeys } from "@/lib/queries/keys"
import { useTemplates } from "@/lib/queries/templates"
import { queryClient } from "@/lib/query-client"
import { toast, toastError } from "@/lib/toast"
import { cn } from "@/lib/utils"

interface RowApplyTemplateDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Slot id to apply the chosen slot-scope template to. */
  slotId: number
  /** Localized slot name shown in the dialog title. */
  slotName: string
  /** Dates to apply the template to — typically the 7 dates of the visible
   *  window. The dialog applies one slot-scope template to every (date, slot)
   *  pair that's not already occupied. */
  dates: string[]
  /** Range invalidation keys so the parent doesn't have to wire toasts itself. */
  rangeFrom: string
  rangeTo: string
  /** Set of "date|slotId" keys for slots already filled — these are skipped to
   *  preserve user data. The dialog reports the count. */
  occupiedKeys: Set<string>
}

export function RowApplyTemplateDialog(props: RowApplyTemplateDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open && <RowApplyTemplateBody key={props.slotId} {...props} />}
    </Dialog>
  )
}

function RowApplyTemplateBody({
  onOpenChange,
  slotId,
  slotName,
  dates,
  rangeFrom,
  rangeTo,
  occupiedKeys,
}: RowApplyTemplateDialogProps) {
  const { t } = useTranslation()
  const templatesQuery = useTemplates("slot")
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [pending, setPending] = useState(false)

  const templates = templatesQuery.data ?? []
  const targetDates = dates.filter((d) => !occupiedKeys.has(`${d}|${slotId}`))
  const overlapCount = dates.length - targetDates.length

  async function handleSubmit() {
    if (!selectedId || targetDates.length === 0) return
    setPending(true)
    try {
      const results = await Promise.allSettled(
        targetDates.map((date) =>
          applyTemplate(selectedId, { date, slot_id: slotId })
        )
      )
      const okCount = results.filter((r) => r.status === "fulfilled").length
      const failCount = results.length - okCount
      void queryClient.invalidateQueries({
        queryKey: plateKeys.range(rangeFrom, rangeTo),
      })
      if (okCount > 0) {
        toast.success(t("planner.row_actions.applied", { count: okCount }))
      }
      if (failCount > 0) {
        const firstError = results.find((r) => r.status === "rejected") as
          | PromiseRejectedResult
          | undefined
        toastError(firstError?.reason, t)
      }
      onOpenChange(false)
    } catch (err) {
      toastError(err, t)
    } finally {
      setPending(false)
    }
  }

  return (
    <DialogContent className="max-w-md">
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          <Bookmark className="size-4" aria-hidden />
          {t("planner.row_actions.apply_dialog_title", { slot: slotName })}
        </DialogTitle>
        <DialogDescription>
          {t("planner.row_actions.apply_dialog_body", {
            count: targetDates.length,
          })}
          {overlapCount > 0 && (
            <span className="mt-1 block text-xs text-on-surface-variant">
              {t("planner.row_actions.apply_skip_occupied", {
                count: overlapCount,
              })}
            </span>
          )}
        </DialogDescription>
      </DialogHeader>

      {templatesQuery.isLoading ? (
        <div className="space-y-2">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </div>
      ) : templates.length === 0 ? (
        <p
          className="rounded-md border border-dashed border-outline-variant/50 bg-surface-container-low/40 px-3 py-6 text-center text-sm text-on-surface-variant"
          data-testid="row-apply-empty"
        >
          {t("planner.row_actions.apply_empty")}
        </p>
      ) : (
        <ul
          className="max-h-64 space-y-1.5 overflow-y-auto pr-1"
          role="list"
          data-testid="row-apply-list"
        >
          {templates.map((tpl) => (
            <li key={tpl.id}>
              <button
                type="button"
                onClick={() => setSelectedId(tpl.id)}
                aria-pressed={selectedId === tpl.id}
                data-testid={`row-apply-item-${tpl.id}`}
                className={cn(
                  "flex w-full items-center justify-between gap-3 rounded-md border bg-surface-container-lowest px-3 py-2 text-left transition-colors",
                  selectedId === tpl.id
                    ? "border-primary/60 bg-primary/8"
                    : "border-outline-variant/40 hover:border-primary/40 hover:bg-surface-container-low"
                )}
              >
                <span className="flex min-w-0 items-center gap-2.5">
                  <Bookmark
                    className="size-3.5 shrink-0 text-secondary"
                    aria-hidden
                  />
                  <span className="truncate text-sm font-medium text-on-surface">
                    {tpl.name}
                  </span>
                </span>
                <Badge variant="secondary" className="shrink-0 text-xs">
                  {countComponents(tpl)}
                </Badge>
              </button>
            </li>
          ))}
        </ul>
      )}

      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          onClick={() => onOpenChange(false)}
        >
          {t("common.cancel")}
        </Button>
        <Button
          type="button"
          onClick={() => void handleSubmit()}
          disabled={!selectedId || targetDates.length === 0 || pending}
          data-testid="row-apply-submit"
        >
          {t("planner.row_actions.apply_submit", { count: targetDates.length })}
        </Button>
      </DialogFooter>
    </DialogContent>
  )
}

function countComponents(tpl: Template): number {
  return tpl.components.length
}
