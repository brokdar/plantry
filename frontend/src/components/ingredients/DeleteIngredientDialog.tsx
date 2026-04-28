import { useState } from "react"
import { Trash2 } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { ApiError } from "@/lib/api/client"
import { useDeleteFood } from "@/lib/queries/foods"

interface DeleteIngredientDialogProps {
  foodId: number
  onDeleted?: () => void
}

export function DeleteIngredientDialog({
  foodId,
  onDeleted,
}: DeleteIngredientDialogProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const mutation = useDeleteFood()

  function handleOpenChange(next: boolean) {
    if (!next) setError(null)
    setOpen(next)
  }

  function confirm() {
    setError(null)
    mutation.mutate(foodId, {
      onSuccess: () => {
        setOpen(false)
        onDeleted?.()
      },
      onError: (err: unknown) => {
        const key = err instanceof ApiError ? err.messageKey : "error.server"
        setError(t(key))
      },
    })
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          className="text-destructive hover:bg-destructive/10 hover:text-destructive"
          data-testid="ingredient-delete"
        >
          <Trash2 className="mr-1.5 size-4" />
          {t("common.delete")}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("ingredient.delete_confirm_title")}</DialogTitle>
          <DialogDescription>
            {t("ingredient.delete_confirm_body")}
          </DialogDescription>
        </DialogHeader>
        {error && <p className="px-1 text-sm text-destructive">{error}</p>}
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            variant="destructive"
            onClick={confirm}
            disabled={mutation.isPending}
            data-testid="confirm-delete"
          >
            {t("common.delete")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
