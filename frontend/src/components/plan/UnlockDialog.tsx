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

interface UnlockDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** The other plates of the batch closure, named ("Monday lunch"). */
  alsoUnlocks: string[]
  pending: boolean
  onUnlock: () => void
}

/**
 * Unlock asks first (D34, UX §6): the plate's ingredients may already be
 * bought, and the whole batch closure returns to draft with it.
 */
export function UnlockDialog({
  open,
  onOpenChange,
  alsoUnlocks,
  pending,
  onUnlock,
}: UnlockDialogProps) {
  const { t, i18n } = useTranslation()
  const list = new Intl.ListFormat(i18n.language, { type: "conjunction" })
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} data-testid="unlock-dialog">
        <DialogHeader>
          <DialogTitle>{t("plan.unlock.title")}</DialogTitle>
          <DialogDescription>{t("plan.unlock.body")}</DialogDescription>
        </DialogHeader>
        {alsoUnlocks.length > 0 && (
          <p className="text-small text-ink">
            {t("plan.unlock.also", { cells: list.format(alsoUnlocks) })}
          </p>
        )}
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
            className="h-11 md:pointer-fine:h-9"
            disabled={pending}
            onClick={onUnlock}
          >
            {t("plan.review.unlock")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
