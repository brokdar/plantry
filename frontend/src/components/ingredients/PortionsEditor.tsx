import { useState } from "react"
import { useTranslation } from "react-i18next"
import { Plus, Trash2 } from "lucide-react"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import {
  usePortions,
  useUpsertPortion,
  useDeletePortion,
} from "@/lib/queries/portions"

interface PortionsEditorProps {
  ingredientId: number
}

export function PortionsEditor({ ingredientId }: PortionsEditorProps) {
  const { t } = useTranslation()
  const { data: portions = [] } = usePortions(ingredientId)
  const upsertMutation = useUpsertPortion()
  const deleteMutation = useDeletePortion()

  const [newUnit, setNewUnit] = useState("")
  const [newGrams, setNewGrams] = useState("")

  function handleAdd(e: React.FormEvent) {
    e.preventDefault()
    const unit = newUnit.trim()
    const grams = Number(newGrams)
    if (!unit || !grams || grams <= 0) return

    upsertMutation.mutate(
      { ingredientId, data: { unit, grams } },
      {
        onSuccess: () => {
          setNewUnit("")
          setNewGrams("")
        },
      }
    )
  }

  function handleDelete(unit: string) {
    deleteMutation.mutate({ ingredientId, unit })
  }

  return (
    <div className="space-y-3">
      <h3 className="text-sm font-medium">{t("portion.title")}</h3>

      {portions.length > 0 && (
        <div className="space-y-2">
          {portions.map((p) => (
            <div
              key={p.unit}
              className="flex items-center gap-3 rounded border px-3 py-2"
            >
              <span className="flex-1 text-sm font-medium">{p.unit}</span>
              <span className="text-sm text-muted-foreground">{p.grams}g</span>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                onClick={() => handleDelete(p.unit)}
                disabled={deleteMutation.isPending}
                aria-label={t("portion.delete_unit", { unit: p.unit })}
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          ))}
        </div>
      )}

      <form onSubmit={handleAdd} className="flex items-end gap-2">
        <div className="flex-1">
          <label className="mb-1 block text-xs text-muted-foreground">
            {t("portion.unit")}
          </label>
          <Input
            value={newUnit}
            onChange={(e) => setNewUnit(e.target.value)}
            placeholder={t("portion.unit_placeholder")}
          />
        </div>
        <div className="w-24">
          <label className="mb-1 block text-xs text-muted-foreground">
            {t("portion.grams")}
          </label>
          <Input
            type="number"
            step="0.1"
            min="0"
            value={newGrams}
            onChange={(e) => setNewGrams(e.target.value)}
          />
        </div>
        <Button
          type="submit"
          size="sm"
          disabled={!newUnit.trim() || !newGrams || upsertMutation.isPending}
        >
          <Plus className="mr-1 size-4" />
          {t("portion.add")}
        </Button>
      </form>
    </div>
  )
}
