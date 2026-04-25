import { useState } from "react"
import { useTranslation } from "react-i18next"
import { Plus, Trash2 } from "lucide-react"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import {
  usePortions,
  useUpsertPortion,
  useDeletePortion,
} from "@/lib/queries/foods"
import { normalizeUnit } from "@/lib/domain/units"

import { UnitLabel, UnitSelect } from "./UnitSelect"

export interface StagedPortion {
  unit: string
  grams: number
}

type PortionsEditorProps =
  | { mode: "bound"; foodId: number }
  | {
      mode: "staged"
      portions: StagedPortion[]
      onChange: (portions: StagedPortion[]) => void
    }

export function PortionsEditor(props: PortionsEditorProps) {
  if (props.mode === "bound") {
    return <BoundPortionsEditor foodId={props.foodId} />
  }
  return (
    <StagedPortionsEditor portions={props.portions} onChange={props.onChange} />
  )
}

function BoundPortionsEditor({ foodId }: { foodId: number }) {
  const { t } = useTranslation()
  const { data } = usePortions(foodId)
  const portions = data?.items ?? []
  const upsertMutation = useUpsertPortion()
  const deleteMutation = useDeletePortion()

  function handleAdd(unit: string, grams: number) {
    upsertMutation.mutate({ foodId, data: { unit, grams } })
  }

  function handleDelete(unit: string) {
    deleteMutation.mutate({ foodId, unit })
  }

  return (
    <PortionsEditorView
      t={t}
      portions={portions}
      onAdd={handleAdd}
      onDelete={handleDelete}
      addPending={upsertMutation.isPending}
      deletePending={deleteMutation.isPending}
    />
  )
}

function StagedPortionsEditor({
  portions,
  onChange,
}: {
  portions: StagedPortion[]
  onChange: (portions: StagedPortion[]) => void
}) {
  const { t } = useTranslation()

  function handleAdd(unit: string, grams: number) {
    const existing = portions.findIndex((p) => normalizeUnit(p.unit) === unit)
    if (existing >= 0) {
      const next = [...portions]
      next[existing] = { unit, grams }
      onChange(next)
    } else {
      onChange([...portions, { unit, grams }])
    }
  }

  function handleDelete(unit: string) {
    onChange(portions.filter((p) => p.unit !== unit))
  }

  return (
    <PortionsEditorView
      t={t}
      portions={portions.map((p) => ({
        food_id: 0,
        unit: p.unit,
        grams: p.grams,
      }))}
      onAdd={handleAdd}
      onDelete={handleDelete}
      addPending={false}
      deletePending={false}
    />
  )
}

interface PortionsEditorViewProps {
  t: ReturnType<typeof useTranslation>["t"]
  portions: Array<{ unit: string; grams: number }>
  onAdd: (unit: string, grams: number) => void
  onDelete: (unit: string) => void
  addPending: boolean
  deletePending: boolean
}

function PortionsEditorView({
  t,
  portions,
  onAdd,
  onDelete,
  addPending,
  deletePending,
}: PortionsEditorViewProps) {
  const [newUnit, setNewUnit] = useState("")
  const [newGrams, setNewGrams] = useState("")

  const usedUnits = portions.map((p) => normalizeUnit(p.unit))

  function handleAdd() {
    const unit = normalizeUnit(newUnit)
    const grams = Number(newGrams)
    if (!unit || !grams || grams <= 0) return
    onAdd(unit, grams)
    setNewUnit("")
    setNewGrams("")
  }

  return (
    <div className="space-y-3" aria-label={t("portion.title")}>
      {portions.length > 0 && (
        <div className="space-y-2">
          {portions.map((p) => (
            <div
              key={p.unit}
              className="flex items-center gap-3 rounded-lg border border-outline-variant/40 bg-surface-container/40 px-3 py-2"
            >
              <UnitLabel unit={p.unit} className="flex-1" />
              <span className="font-mono text-xs text-on-surface-variant tabular-nums">
                {p.grams} g
              </span>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                onClick={() => onDelete(p.unit)}
                disabled={deletePending}
                aria-label={t("portion.delete_unit", { unit: p.unit })}
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          ))}
        </div>
      )}

      <div className="flex items-end gap-2">
        <div className="flex-1">
          <label className="mb-1 block text-xs text-muted-foreground">
            {t("portion.unit")}
          </label>
          <UnitSelect
            value={newUnit}
            onValueChange={setNewUnit}
            excludeKeys={usedUnits}
            placeholder={t("portion.unit_placeholder_select", {
              defaultValue: t("unit.placeholder"),
            })}
            testId="portion-unit"
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
            data-testid="portion-grams"
            value={newGrams}
            onChange={(e) => setNewGrams(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault()
                handleAdd()
              }
            }}
          />
        </div>
        <Button
          type="button"
          size="sm"
          onClick={handleAdd}
          disabled={!newUnit.trim() || !newGrams || addPending}
        >
          <Plus className="mr-1 size-4" />
          {t("portion.add")}
        </Button>
      </div>
    </div>
  )
}
