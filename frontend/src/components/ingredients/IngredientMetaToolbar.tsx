import { Loader2, RefreshCw } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { toast, toastError } from "@/lib/toast"
import { useRefetchFood, useSyncPortions } from "@/lib/queries/foods"
import type { LeafFood } from "@/lib/api/foods"
import { cn } from "@/lib/utils"

const SOURCE_DOT: Record<string, string> = {
  fdc: "bg-primary",
  off: "bg-tertiary",
  manual: "bg-on-surface-variant/50",
}

interface IngredientMetaToolbarProps {
  ingredient: LeafFood
  disabled?: boolean
  /**
   * Called with the refetched ingredient so the parent can reset its form to
   * the freshly-sourced nutrient values.
   */
  onRefetched: (updated: LeafFood) => void
}

export function IngredientMetaToolbar({
  ingredient,
  disabled,
  onRefetched,
}: IngredientMetaToolbarProps) {
  const { t } = useTranslation()
  const refetchMutation = useRefetchFood()
  const syncPortionsMutation = useSyncPortions()

  const canRefetch =
    (!!ingredient.barcode && ingredient.barcode.length > 0) ||
    (!!ingredient.fdc_id && ingredient.fdc_id.length > 0)
  const canSyncPortions = !!ingredient.fdc_id && ingredient.fdc_id.length > 0
  const source = ingredient.source ?? "manual"

  async function handleRefetch() {
    if (!canRefetch) return
    try {
      const updated = await refetchMutation.mutateAsync({
        id: ingredient.id,
        lang: undefined,
      })
      onRefetched(updated as LeafFood)
    } catch (err) {
      toastError(err, t)
    }
  }

  async function handleSyncPortions() {
    try {
      const result = await syncPortionsMutation.mutateAsync(ingredient.id)
      toast.success(t("ingredient.sync_portions_done", { count: result.added }))
    } catch (err) {
      toastError(err, t)
    }
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Badge variant="secondary" className="gap-1.5">
        <span
          className={cn(
            "inline-block size-1.5 rounded-full",
            SOURCE_DOT[source] ?? "bg-on-surface-variant/50"
          )}
          aria-hidden
        />
        {t(`ingredient.source_${source}`, { defaultValue: source })}
      </Badge>
      {canRefetch && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-7 gap-1.5 text-xs"
          onClick={handleRefetch}
          disabled={refetchMutation.isPending || disabled}
          data-testid="ingredient-refetch"
        >
          {refetchMutation.isPending ? (
            <Loader2 className="size-3.5 animate-spin" aria-hidden />
          ) : (
            <RefreshCw className="size-3.5" aria-hidden />
          )}
          {t("ingredient.refetch")}
        </Button>
      )}
      {canSyncPortions && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-7 gap-1.5 text-xs"
          onClick={handleSyncPortions}
          disabled={syncPortionsMutation.isPending || disabled}
          data-testid="ingredient-sync-portions"
        >
          {syncPortionsMutation.isPending ? (
            <Loader2 className="size-3.5 animate-spin" aria-hidden />
          ) : (
            <RefreshCw className="size-3.5" aria-hidden />
          )}
          {t("ingredient.sync_portions")}
        </Button>
      )}
    </div>
  )
}
