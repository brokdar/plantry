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

interface DiscardDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Draft plates the discard deletes. */
  plates: number
  pending: boolean
  onDiscard: () => void
}

/** Discard draft asks first (UX §6 destructive confirmation); the toast
 *  after it still offers Undo. */
export function DiscardDialog({
  open,
  onOpenChange,
  plates,
  pending,
  onDiscard,
}: DiscardDialogProps) {
  const { t } = useTranslation()
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} data-testid="discard-dialog">
        <DialogHeader>
          <DialogTitle>{t("plan.discard.title")}</DialogTitle>
          <DialogDescription>
            {t("plan.discard.body", { count: plates })}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            className="h-11 md:pointer-fine:h-9"
            onClick={() => onOpenChange(false)}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            className="h-11 md:pointer-fine:h-9"
            disabled={pending}
            onClick={onDiscard}
          >
            {t("plan.review.discard")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
