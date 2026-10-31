import { useState } from "react"
import { useTranslation } from "react-i18next"

import { BatchBadge } from "@/components/batch/BatchBadge"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import type { Plan, PlanCell } from "@/lib/api/plan"
import type { Session } from "@/lib/api/sessions"
import { colourBatches } from "@/lib/plan/batch-colours"
import { weekdayShort } from "@/lib/plan/format"
import { usePresets, useApplyPreset, useUndoApply } from "@/lib/queries/presets"
import {
  useMarkNoPlate,
  usePlanMeal,
  useSetNoPlateNote,
} from "@/lib/queries/plan"
import { useCreatePlate } from "@/lib/queries/plates"
import { showPresetApplyToasts } from "@/lib/preset-apply-toast"

import type { PickerTarget } from "./FoodPicker"

interface CellSheetProps {
  /** The cell without a plate. */
  cell: PlanCell
  plan: Plan
  onPick: (target: PickerTarget) => void
  onClose: () => void
}

const actionClass = "h-11 justify-start md:pointer-fine:h-9"

/**
 * The sheet for a cell without a plate (UX §5.5). A plan or routine cell
 * offers Add food, Use a routine, Eat a leftover and Mark as No plate; a
 * No plate cell offers Plan this meal and its note.
 */
export function CellSheet({ cell, plan, onPick, onClose }: CellSheetProps) {
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-4 pb-6">
      {cell.mode === "none" ? (
        <NoPlateActions cell={cell} />
      ) : (
        <EmptyActions
          cell={cell}
          plan={plan}
          onPick={onPick}
          onClose={onClose}
        />
      )}
    </div>
  )
}

function EmptyActions({ cell, plan, onPick, onClose }: CellSheetProps) {
  const { t } = useTranslation()
  const markNoPlate = useMarkNoPlate()
  return (
    <>
      {cell.mode === "routine" && (
        <p className="text-small text-ink-muted">
          {cell.preset_name
            ? t("plan.cell.routine_named", { name: cell.preset_name })
            : t("plan.cell.routine")}
        </p>
      )}
      <Button
        type="button"
        className={actionClass}
        onClick={() =>
          onPick({ kind: "create", date: cell.date, slotId: cell.slot_id })
        }
      >
        {t("plan.cell_sheet.add_food")}
      </Button>
      <Leftovers cell={cell} plan={plan} onDone={onClose} />
      <Routines cell={cell} onDone={onClose} />
      <Button
        type="button"
        variant="outline"
        className={actionClass}
        onClick={() => {
          markNoPlate.mutate(cell)
          onClose()
        }}
      >
        {t("plan.sheet.mark_no_plate")}
      </Button>
    </>
  )
}

/** Cell order for "is this cell after the cook cell". */
function slotOrder(plan: Plan) {
  const order = new Map(plan.slots.map((s) => [s.id, [s.sort_order, s.id]]))
  return (date: string, slotId: number) => {
    const [sort, id] = order.get(slotId) ?? [0, slotId]
    return `${date}|${String(sort).padStart(6, "0")}|${String(id).padStart(9, "0")}`
  }
}

/** Batches in the window with portions to spare whose use-by covers this
 *  cell, and which cook before it. */
function eatableLeftovers(cell: PlanCell, plan: Plan): Session[] {
  const key = slotOrder(plan)
  const here = key(cell.date, cell.slot_id)
  return plan.sessions.filter(
    (s) =>
      s.surplus > 0 &&
      s.use_by >= cell.date &&
      key(s.cook_date, s.cook_slot_id) < here
  )
}

function Leftovers({
  cell,
  plan,
  onDone,
}: {
  cell: PlanCell
  plan: Plan
  onDone: () => void
}) {
  const { t, i18n } = useTranslation()
  const createPlate = useCreatePlate()
  const sessions = eatableLeftovers(cell, plan)
  if (sessions.length === 0) return null
  const colours = new Map(
    colourBatches(plan.sessions, plan.slots).map((b) => [
      b.session.id,
      b.colour,
    ])
  )
  return (
    <section aria-labelledby="cell-leftovers" className="flex flex-col gap-2">
      <h3
        id="cell-leftovers"
        className="font-sans text-small font-semibold text-ink"
      >
        {t("plan.cell_sheet.eat_leftover")}
      </h3>
      <ul className="flex flex-col">
        {sessions.map((s) => (
          <li key={s.id}>
            <button
              type="button"
              data-testid="cell-leftover"
              disabled={createPlate.isPending}
              onClick={() =>
                createPlate.mutate(
                  {
                    date: cell.date,
                    slot_id: cell.slot_id,
                    components: [
                      {
                        food_id: s.food_id,
                        portions: 1,
                        cook_session_id: s.id,
                      },
                    ],
                  },
                  { onSuccess: onDone }
                )
              }
              className="flex min-h-11 w-full items-center gap-3 rounded-cell px-2 text-left hover:bg-panel-sunken focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
            >
              {colours.get(s.id) && (
                <BatchBadge
                  sessionId={s.id}
                  colour={colours.get(s.id)!}
                  variant="surplus"
                  count={s.surplus}
                />
              )}
              <span className="text-body text-ink">
                {t("plan.cell_sheet.leftover_item", {
                  name: s.food_name,
                  weekday: weekdayShort(s.cook_date, i18n),
                })}
              </span>
            </button>
          </li>
        ))}
      </ul>
    </section>
  )
}

function Routines({ cell, onDone }: { cell: PlanCell; onDone: () => void }) {
  const { t } = useTranslation()
  const presets = usePresets({
    slot_ids: [cell.slot_id],
    sort: "recent",
    limit: 20,
  })
  const apply = useApplyPreset()
  const undo = useUndoApply()
  const items = presets.data?.items ?? []
  return (
    <section aria-labelledby="cell-routines" className="flex flex-col gap-2">
      <h3
        id="cell-routines"
        className="font-sans text-small font-semibold text-ink"
      >
        {t("plan.cell_sheet.use_routine")}
      </h3>
      {presets.isSuccess && items.length === 0 && (
        <p className="text-small text-ink-muted">
          {t("plan.cell_sheet.no_routines")}
        </p>
      )}
      <ul className="flex flex-col">
        {items.map((preset) => (
          <li key={preset.id}>
            <button
              type="button"
              data-testid="cell-routine"
              disabled={apply.isPending}
              onClick={() =>
                apply.mutate(
                  {
                    presetId: preset.id,
                    input: {
                      target_date: cell.date,
                      on_conflict: "skip",
                      slot_ids_filter: [cell.slot_id],
                    },
                  },
                  {
                    onSuccess: (result) => {
                      showPresetApplyToasts(result, t, (r) => undo.mutate(r))
                      onDone()
                    },
                  }
                )
              }
              className="flex min-h-11 w-full items-center rounded-cell px-2 text-left text-body text-ink hover:bg-panel-sunken focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
            >
              {preset.name}
            </button>
          </li>
        ))}
      </ul>
    </section>
  )
}

function NoPlateActions({ cell }: { cell: PlanCell }) {
  const { t } = useTranslation()
  const planMeal = usePlanMeal()
  const setNote = useSetNoPlateNote()
  const [note, setNoteDraft] = useState(cell.note ?? "")
  const trimmed = note.trim() || null
  return (
    <>
      <p className="text-small text-ink-muted italic">
        {t("plan.cell.no_plate")}
      </p>
      <Button
        type="button"
        className={actionClass}
        disabled={planMeal.isPending}
        onClick={() => planMeal.mutate(cell)}
      >
        {t("plan.cell_sheet.plan_meal")}
      </Button>
      <form
        className="flex flex-col gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          setNote.mutate({ cell, note: trimmed })
        }}
      >
        <label className="flex flex-col gap-1">
          <span className="font-sans text-small font-semibold text-ink">
            {t("plan.cell_sheet.note")}
          </span>
          <Textarea
            value={note}
            onChange={(e) => setNoteDraft(e.target.value)}
            rows={2}
            placeholder={t("plan.cell_sheet.note_placeholder")}
            data-testid="cell-note"
          />
        </label>
        <Button
          type="submit"
          variant="outline"
          className="h-11 self-start md:pointer-fine:h-9"
          disabled={trimmed === cell.note || setNote.isPending}
        >
          {t("plan.cell_sheet.edit_note")}
        </Button>
      </form>
    </>
  )
}
