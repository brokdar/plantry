import { useNavigate } from "@tanstack/react-router"
import { Copy } from "lucide-react"
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
import { useCopyWeek } from "@/lib/queries/weeks"
import { toastError, toastSuccess } from "@/lib/toast"
import { currentYearWeek } from "@/lib/weeks-util"

type CopyToCurrentButtonProps = {
  weekId: number
  size?: "default" | "sm"
  testId?: string
}

export function CopyToCurrentButton({
  weekId,
  size = "default",
  testId,
}: CopyToCurrentButtonProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const copyMut = useCopyWeek()
  const [open, setOpen] = useState(false)
  const target = currentYearWeek()

  async function handleConfirm() {
    try {
      await copyMut.mutateAsync({
        id: weekId,
        input: { target_year: target.year, target_week: target.week },
      })
      toastSuccess("archive.copy_success", t)
      setOpen(false)
      await navigate({ to: "/" })
    } catch (err) {
      toastError(err, t)
    }
  }

  return (
    <>
      <Button
        type="button"
        size={size}
        onClick={(e) => {
          e.preventDefault()
          e.stopPropagation()
          setOpen(true)
        }}
        data-testid={testId ?? `copy-to-current-${weekId}`}
      >
        <Copy className="mr-1.5 size-4" aria-hidden />
        {t("archive.copy_to_current")}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("archive.copy_confirm_title")}</DialogTitle>
            <DialogDescription>
              {t("archive.copy_confirm_body", {
                week: target.week,
                year: target.year,
              })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              onClick={handleConfirm}
              disabled={copyMut.isPending}
              data-testid="confirm-copy-to-current"
            >
              {t("archive.copy_to_current")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
