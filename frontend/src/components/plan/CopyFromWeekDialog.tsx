import { CalendarRange } from "lucide-react"
import { useId, useState } from "react"
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
import { Input } from "@/components/ui/input"
import type { ApplyConflict } from "@/lib/api/presets"
import { useApplyResultToasts, useCopyWeek } from "@/lib/queries/presets"

interface CopyFromWeekDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** First day the copy lands on. */
  targetStart: string
  /** Seeds the source picker, usually the previous window's start. */
  defaultSourceStart: string
}

/** Copy week (B2): copies a past week's plates day by day onto
 *  `targetStart`, then shows the shared apply toasts with Undo. */
export function CopyFromWeekDialog({
  open,
  onOpenChange,
  targetStart,
  defaultSourceStart,
}: CopyFromWeekDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="bg-panel text-ink"
        data-testid="copy-from-week-dialog"
      >
        {/* Re-keyed per opening so the form starts fresh each time. */}
        {open && (
          <DialogBody
            key={defaultSourceStart}
            defaultSourceStart={defaultSourceStart}
            targetStart={targetStart}
            onClose={() => onOpenChange(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

function DialogBody({
  defaultSourceStart,
  targetStart,
  onClose,
}: {
  defaultSourceStart: string
  targetStart: string
  onClose: () => void
}) {
  const { t } = useTranslation()
  const sourceId = useId()
  const [sourceStart, setSourceStart] = useState(defaultSourceStart)
  const [conflict, setConflict] = useState<ApplyConflict>("skip")
  const copyWeek = useCopyWeek()
  const showResult = useApplyResultToasts()

  function submit() {
    copyWeek.mutate(
      {
        source_start: sourceStart,
        target_start: targetStart,
        on_conflict: conflict,
      },
      {
        onSuccess: (result) => {
          showResult(result)
          onClose()
        },
      }
    )
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2 text-h3">
          <CalendarRange className="size-4" strokeWidth={1.5} aria-hidden />
          {t("plan.copy_week.title")}
        </DialogTitle>
        <DialogDescription className="text-small text-ink-muted">
          {t("plan.copy_week.body")}
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-1.5">
          <label htmlFor={sourceId} className="text-small text-ink-muted">
            {t("plan.copy_week.source_label")}
          </label>
          <Input
            id={sourceId}
            type="date"
            value={sourceStart}
            onChange={(e) => setSourceStart(e.target.value)}
            className="pointer-coarse:h-11"
          />
        </div>

        <fieldset className="flex flex-col gap-1">
          <legend className="mb-1 text-small text-ink-muted">
            {t("routines.apply.conflict_label")}
          </legend>
          {(["skip", "overwrite"] as const).map((value) => (
            <label
              key={value}
              className="flex min-h-11 items-center gap-2 text-body text-ink md:min-h-9"
            >
              <input
                type="radio"
                name="copy-week-conflict"
                value={value}
                checked={conflict === value}
                onChange={() => setConflict(value)}
                className="size-4 accent-accent"
              />
              {t(`routines.apply.conflict_${value}`)}
            </label>
          ))}
        </fieldset>
      </div>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          {t("common.cancel")}
        </Button>
        <Button
          type="button"
          onClick={submit}
          disabled={copyWeek.isPending || !sourceStart}
        >
          {t("plan.copy_week.submit")}
        </Button>
      </DialogFooter>
    </>
  )
}
