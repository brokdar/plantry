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

interface ConfirmRemovalProps {
  /** The confirmed plates Next would remove, named ("Thursday dinner:
   *  Chili"). Empty = closed. */
  plates: string[]
  pending: boolean
  onCancel: () => void
  onConfirm: () => void
}

/**
 * Marking a meal with a confirmed plate No plate removes the plate, whose
 * ingredients may be bought already, so it asks first (D34, UX §6). The
 * removal itself offers Undo afterwards.
 */
export function ConfirmRemoval({
  plates,
  pending,
  onCancel,
  onConfirm,
}: ConfirmRemovalProps) {
  const { t } = useTranslation()
  return (
    <Dialog
      open={plates.length > 0}
      onOpenChange={(open) => !open && onCancel()}
    >
      <DialogContent
        showCloseButton={false}
        data-testid="wizard-confirm-removal"
      >
        <DialogHeader>
          <DialogTitle>
            {t("plan.wizard.remove.title", { count: plates.length })}
          </DialogTitle>
          <DialogDescription>{t("plan.wizard.remove.body")}</DialogDescription>
        </DialogHeader>
        <ul className="flex flex-col gap-1 text-small text-ink">
          {plates.map((p) => (
            <li key={p}>{p}</li>
          ))}
        </ul>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            className="h-11 md:pointer-fine:h-9"
            onClick={onCancel}
          >
            {t("plan.wizard.remove.keep")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            className="h-11 md:pointer-fine:h-9"
            disabled={pending}
            onClick={onConfirm}
          >
            {t("plan.wizard.remove.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
