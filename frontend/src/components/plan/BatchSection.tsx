import { useState } from "react"
import { useTranslation } from "react-i18next"

import { BatchBadge } from "@/components/batch/BatchBadge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import type { Cell, Session } from "@/lib/api/sessions"
import type { BatchColour } from "@/lib/plan/batch-colours"
import {
  batchBadgeText,
  coverageText,
  formatDate,
  weekdayShort,
} from "@/lib/plan/format"
import {
  useAddLeftovers,
  useDeleteSession,
  useLeftoverCells,
  useUpdateSession,
} from "@/lib/queries/sessions"

import { QuantityStepper } from "./QuantityStepper"

const MIN_MEALS = 2
const MAX_MEALS = 7
/** The server's cap on suggested leftover cells (plate.MaxLeftoverCells). */
const MAX_LEFTOVER_CELLS = 14

interface BatchSectionProps {
  session: Session
  /** The plate component this section belongs to. */
  componentId: number
  colour: BatchColour | null
  /** Slot id → display name ("dinner"). */
  slotName: (slotId: number) => string
  /** `plan.freezing_enabled`: shows the freeze stepper. */
  freezing: boolean
  readOnly: boolean
  /** Opens another cell's sheet (the batch's cook cell). */
  onOpenCell: (key: string) => void
  /** Closes the sheet once the batch is removed. */
  onClose: () => void
}

const cellKey = (c: Cell) => `${c.date}|${c.slot_id}`

/**
 * The batch controls for one composed component (UX §5.5, D29): on a fresh
 * single meal "Cook as a batch", on the cook cell of a batch its portions,
 * freezing, use-by and Remove batch, and on a leftover where it comes from.
 */
export function BatchSection(props: BatchSectionProps) {
  const link = props.session.links.find(
    (l) => l.component_id === props.componentId
  )
  if (!link) return null
  if (link.kind !== "fresh") return <LeftoverInfo {...props} link={link} />
  if (props.readOnly) return null
  if (props.session.meals === 1) return <CookAsBatch {...props} />
  return <BatchControls {...props} />
}

function SectionFrame({ children }: { children: React.ReactNode }) {
  return (
    <div
      className="flex flex-col gap-3 rounded-cell border border-line p-3"
      data-testid="batch-section"
    >
      {children}
    </div>
  )
}

function LeftoverInfo({
  session,
  colour,
  onOpenCell,
  link,
}: BatchSectionProps & { link: Session["links"][number] }) {
  const { t, i18n } = useTranslation()
  return (
    <SectionFrame>
      <div className="flex flex-wrap items-center gap-2">
        {colour && (
          <BatchBadge
            sessionId={session.id}
            colour={colour}
            variant={link.kind === "frozen" ? "frozen" : "leftover"}
            position={link.position}
            meals={session.meals}
          />
        )}
        <span className="text-small text-ink">
          {t("plan.batch.from", {
            weekday: weekdayShort(session.cook_date, i18n),
            name: session.food_name,
            position: link.position,
            meals: session.meals,
          })}
        </span>
      </div>
      <Button
        type="button"
        variant="outline"
        className="h-11 self-start md:pointer-fine:h-9"
        onClick={() =>
          onOpenCell(
            cellKey({ date: session.cook_date, slot_id: session.cook_slot_id })
          )
        }
      >
        {t("plan.batch.open")}
      </Button>
    </SectionFrame>
  )
}

function CookAsBatch({ session, slotName }: BatchSectionProps) {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState(false)
  // How many suggested cells to show, ticked or not.
  const [requested, setRequested] = useState(MIN_MEALS - 1)
  // Unticked suggestions; everything the server suggests starts ticked.
  const [unticked, setUnticked] = useState<Set<string>>(new Set())
  const suggested = useLeftoverCells(open ? session.id : 0, requested)
  const addLeftovers = useAddLeftovers()

  if (!open) {
    return (
      <SectionFrame>
        <Button
          type="button"
          variant="outline"
          className="h-11 self-start md:pointer-fine:h-9"
          onClick={() => setOpen(true)}
        >
          {t("plan.batch.cook_as_batch")}
        </Button>
      </SectionFrame>
    )
  }

  const cells = suggested.data?.cells ?? []
  const chosen = cells.filter((c) => !unticked.has(cellKey(c)))
  const fresh = { date: session.cook_date, slot_id: session.cook_slot_id }
  // Meals is always the cook meal plus the ticked cells (while the cells
  // load, what they will be).
  const meals = suggested.data
    ? 1 + chosen.length
    : 1 + requested - unticked.size

  /** + shows the next suggested cell, ticked; − drops the last ticked cell
   *  (and the unticked ones after it). Unticks before it are kept. */
  function changeMeals(next: number) {
    if (next > meals) {
      // Fewer cells than asked for: the server has no more free cells.
      if (suggested.data && cells.length < requested) return
      setRequested((r) => Math.min(MAX_LEFTOVER_CELLS, r + 1))
      return
    }
    let last = cells.length - 1
    while (last >= 0 && unticked.has(cellKey(cells[last]!))) last--
    if (last < 1) return
    const kept = new Set(cells.slice(0, last).map(cellKey))
    setRequested(last)
    setUnticked((prev) => new Set([...prev].filter((k) => kept.has(k))))
  }

  function reset() {
    setRequested(MIN_MEALS - 1)
    setUnticked(new Set())
  }

  function toggle(c: Cell) {
    setUnticked((prev) => {
      const next = new Set(prev)
      if (next.has(cellKey(c))) next.delete(cellKey(c))
      else next.add(cellKey(c))
      return next
    })
  }

  return (
    <SectionFrame>
      <div className="flex items-center justify-between gap-3">
        <span className="text-small text-ink">{t("plan.batch.meals")}</span>
        <QuantityStepper
          label={t("plan.batch.meals")}
          value={meals}
          min={MIN_MEALS}
          max={MAX_MEALS}
          step={1}
          onChange={changeMeals}
        />
      </div>
      <fieldset className="flex flex-col">
        <legend className="mb-1 text-caption text-ink-muted">
          {t("plan.batch.leftover_meals")}
        </legend>
        {suggested.isSuccess && cells.length === 0 && (
          <p className="text-small text-ink-muted">
            {t("plan.batch.no_cells")}
          </p>
        )}
        {cells.map((c) => (
          <label
            key={cellKey(c)}
            className="flex min-h-11 cursor-pointer items-center gap-3 text-body text-ink md:pointer-fine:min-h-9"
          >
            <input
              type="checkbox"
              checked={!unticked.has(cellKey(c))}
              onChange={() => toggle(c)}
              className="size-4 accent-accent"
            />
            {t("batch.cell", {
              weekday: weekdayShort(c.date, i18n),
              slot: slotName(c.slot_id),
            })}
          </label>
        ))}
      </fieldset>
      <p className="text-small text-ink-muted" data-testid="batch-coverage">
        {coverageText([fresh, ...chosen], slotName, i18n)}
      </p>
      <div className="flex gap-2">
        <Button
          type="button"
          variant="outline"
          className="h-11 md:pointer-fine:h-9"
          onClick={() => {
            // A new start next time: the default meals, every cell ticked.
            setOpen(false)
            reset()
          }}
        >
          {t("common.cancel")}
        </Button>
        <Button
          type="button"
          className="h-11 flex-1 md:pointer-fine:h-9"
          disabled={chosen.length === 0 || addLeftovers.isPending}
          onClick={() =>
            addLeftovers.mutate(
              { id: session.id, cells: chosen },
              { onSuccess: () => setOpen(false) }
            )
          }
        >
          {t("plan.batch.create")}
        </Button>
      </div>
    </SectionFrame>
  )
}

function BatchControls({
  session,
  slotName,
  freezing,
  onClose,
}: BatchSectionProps) {
  const { t, i18n } = useTranslation()
  const update = useUpdateSession()
  const remove = useDeleteSession()
  const [pending, setPending] = useState<{
    portions: number
    frozen: number
  } | null>(null)
  const [confirm, setConfirm] = useState(false)

  const portions = pending?.portions ?? session.portions
  const frozen = pending?.frozen ?? session.frozen_portions
  const spare = Math.max(0, portions - session.consumed)

  function save(next: { portions: number; frozen: number }) {
    const value = {
      portions: next.portions,
      frozen: Math.min(next.frozen, next.portions),
    }
    setPending(value)
    update.mutate(
      {
        id: session.id,
        input: {
          portions: value.portions,
          frozen_portions: value.frozen,
          note: session.note,
        },
      },
      { onSettled: () => setPending(null) }
    )
  }

  return (
    <SectionFrame>
      <div className="flex items-center justify-between gap-3">
        <span className="text-small text-ink">{t("plan.batch.portions")}</span>
        {/* Named after the food: a plate with a main and a side shows two
            batch blocks. */}
        <QuantityStepper
          label={t("plan.batch.portions_named", { name: session.food_name })}
          value={portions}
          min={1}
          max={100}
          step={1}
          onChange={(p) => save({ portions: p, frozen })}
          testId="batch-portions"
        />
      </div>
      {freezing && (
        <div className="flex items-center justify-between gap-3">
          <span className="text-small text-ink">{t("plan.batch.freeze")}</span>
          <QuantityStepper
            label={t("plan.batch.freeze_named", { name: session.food_name })}
            value={frozen}
            min={0}
            max={portions}
            step={1}
            onChange={(f) => save({ portions, frozen: f })}
          />
        </div>
      )}
      <p className="text-small text-ink-muted" data-testid="batch-coverage">
        {coverageText(session.links, slotName, i18n)}
        {spare > 0 &&
          ` · ${batchBadgeText({ variant: "surplus", count: spare }, i18n)}`}
      </p>
      <p className="text-small text-ink-muted">
        {t("plan.batch.use_by", {
          date: formatDate(session.use_by, i18n, {
            weekday: "short",
            day: "numeric",
            month: "short",
          }),
        })}
      </p>
      <Button
        type="button"
        variant="outline"
        className="h-11 self-start text-danger md:pointer-fine:h-9"
        aria-label={t("plan.batch.remove_named", { name: session.food_name })}
        onClick={() => setConfirm(true)}
      >
        {t("plan.batch.remove")}
      </Button>
      <Dialog open={confirm} onOpenChange={setConfirm}>
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>{t("plan.batch.remove_title")}</DialogTitle>
            <DialogDescription>
              {t("plan.batch.remove_body", { count: session.meals - 1 })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              className="h-11 md:pointer-fine:h-9"
              onClick={() => setConfirm(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="button"
              variant="destructive"
              className="h-11 md:pointer-fine:h-9"
              disabled={remove.isPending}
              onClick={() =>
                remove.mutate(session.id, {
                  onSuccess: () => {
                    setConfirm(false)
                    onClose()
                  },
                })
              }
            >
              {t("plan.batch.remove")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SectionFrame>
  )
}
