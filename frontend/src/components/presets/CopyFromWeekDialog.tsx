import { CalendarRange } from "lucide-react"
import { useState } from "react"
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
import { useCopyWeek, useUndoApply } from "@/lib/queries/presets"
import { showPresetApplyToasts } from "@/lib/preset-apply-toast"
import { toastError } from "@/lib/toast"

interface CopyFromWeekDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Target Monday of the week the copy lands on. */
  targetStart: string
  /** Default source start to seed the picker. Caller usually passes the
   *  previous week's Monday. */
  defaultSourceStart: string
}

export function CopyFromWeekDialog({
  open,
  onOpenChange,
  targetStart,
  defaultSourceStart,
}: CopyFromWeekDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="copy-from-week-dialog">
        {/* Re-key on (open, defaultSourceStart) so the form state resets
            every time the dialog reopens, without a setState-in-effect. */}
        {open && (
          <DialogBody
            key={`${defaultSourceStart}-${open ? "open" : "closed"}`}
            defaultSourceStart={defaultSourceStart}
            targetStart={targetStart}
            onClose={() => onOpenChange(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

interface DialogBodyProps {
  defaultSourceStart: string
  targetStart: string
  onClose: () => void
}

function DialogBody({
  defaultSourceStart,
  targetStart,
  onClose,
}: DialogBodyProps) {
  const { t } = useTranslation()
  const [sourceStart, setSourceStart] = useState(defaultSourceStart)
  const [conflict, setConflict] = useState<"skip" | "overwrite">("skip")

  const copyMutation = useCopyWeek()
  const undoMutation = useUndoApply()

  function handleSubmit() {
    copyMutation.mutate(
      {
        source_start: sourceStart,
        target_start: targetStart,
        on_conflict: conflict,
      },
      {
        onSuccess: (result) => {
          showPresetApplyToasts(result, t, (snap) =>
            undoMutation.mutate(snap, {
              onError: (err) => toastError(err, t),
            })
          )
          onClose()
        },
        onError: (err) => toastError(err, t),
      }
    )
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          <CalendarRange className="size-4" aria-hidden />
          {t("preset.copy_week.title")}
        </DialogTitle>
        <DialogDescription>{t("preset.copy_week.body")}</DialogDescription>
      </DialogHeader>

      <div className="space-y-4">
        <label className="flex flex-col gap-1.5 text-sm">
          <span className="font-body text-[11px] font-bold tracking-widest text-muted-foreground uppercase">
            {t("preset.copy_week.source_label")}
          </span>
          <Input
            type="date"
            value={sourceStart}
            onChange={(e) => setSourceStart(e.target.value)}
          />
        </label>

        <fieldset className="space-y-2">
          <legend className="font-body text-[11px] font-bold tracking-widest text-muted-foreground uppercase">
            {t("preset.apply.conflict_label")}
          </legend>
          <div className="space-y-1">
            <label className="flex items-center gap-2 text-sm">
              <input
                type="radio"
                name="conflict"
                value="skip"
                checked={conflict === "skip"}
                onChange={() => setConflict("skip")}
              />
              {t("preset.apply.conflict_skip")}
            </label>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="radio"
                name="conflict"
                value="overwrite"
                checked={conflict === "overwrite"}
                onChange={() => setConflict("overwrite")}
              />
              {t("preset.apply.conflict_overwrite")}
            </label>
          </div>
        </fieldset>
      </div>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          {t("common.cancel")}
        </Button>
        <Button
          type="button"
          onClick={handleSubmit}
          disabled={copyMutation.isPending || !sourceStart}
        >
          <CalendarRange className="size-4" />
          {t("preset.copy_week.submit")}
        </Button>
      </DialogFooter>
    </>
  )
}
