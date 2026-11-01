import {
  ArrowLeftRight,
  CalendarSync,
  Check,
  Lock,
  LockOpen,
  Plus,
  RefreshCw,
  Shuffle,
  TriangleAlert,
  X,
} from "lucide-react"
import { useCallback, useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { BatchBadge } from "@/components/batch/BatchBadge"
import {
  QuantityUnitInput,
  type QuantityUnitValue,
} from "@/components/component/QuantityUnitInput"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import type { PlateFeedbackStatus } from "@/lib/api/feedback"
import type { RegenerateScope } from "@/lib/api/generation"
import { KNOWN_ISSUE_CODES, type Plan } from "@/lib/api/plan"
import type { Plate, PlateComponent } from "@/lib/api/plates"
import type { Session, SessionWarning } from "@/lib/api/sessions"
import { todayISO } from "@/lib/dates"
import { colourBatches } from "@/lib/plan/batch-colours"
import { closureCells, type BoardCell } from "@/lib/plan/board"
import { formatAmount, formatDate, formatNumber } from "@/lib/plan/format"
import { plateMacros } from "@/lib/plan/plate-macros"
import { useClearFeedback, useRecordFeedback } from "@/lib/queries/feedback"
import { useFood, useFoodMacros } from "@/lib/queries/foods"
import { useRegenerate } from "@/lib/queries/generation"
import { useConfirm, useUnlock } from "@/lib/queries/plan"
import {
  useDeletePlate,
  useRemoveComponent,
  useUpdateComponent,
  useUpdatePlate,
} from "@/lib/queries/plates"
import { useCreatePreset } from "@/lib/queries/presets"
import { useUpdateSession } from "@/lib/queries/sessions"
import { toast } from "@/lib/toast"
import { cn } from "@/lib/utils"

import { BatchSection } from "./BatchSection"
import type { PickerTarget } from "./FoodPicker"
import { QuantityStepper } from "./QuantityStepper"
import { UnlockDialog } from "./UnlockDialog"

const FEEDBACK: PlateFeedbackStatus[] = ["loved", "disliked", "not_eaten"]

interface PlateSheetProps {
  cell: BoardCell & { plate: Plate }
  plan: Plan
  /** Past windows: only feedback. */
  readOnly: boolean
  /** A generation runs for this window: regenerating waits for it. */
  running: boolean
  /** `plan.freezing_enabled`. */
  freezing: boolean
  slotName: (slotId: number) => string
  onPick: (target: PickerTarget) => void
  onOpenCell: (key: string) => void
  /** Mark as No plate (a confirmed plate goes through the re-plan). */
  onMarkNoPlate: (cell: BoardCell) => void
  onClose: () => void
}

function sorted(plate: Plate) {
  return [...plate.components].sort((a, b) => a.sort_order - b.sort_order)
}

/**
 * The plate sheet (UX §5.5, D33): the plate's issues, "Why this?", the
 * components with their quantities, swap and remove; eaters; batch controls
 * per recipe; macros for one eater; note; feedback; the review actions
 * (Confirm or Unlock, Replace, Different side, Regenerate day); and Mark as
 * No plate, Save as routine and Delete plate.
 */
export function PlateSheet({
  cell,
  plan,
  readOnly,
  running,
  freezing,
  slotName,
  onPick,
  onOpenCell,
  onMarkNoPlate,
  onClose,
}: PlateSheetProps) {
  const { t } = useTranslation()
  const plate = cell.plate
  const components = sorted(plate)
  const colours = new Map(
    colourBatches(plan.sessions, plan.slots).map((b) => [
      b.session.id,
      b.colour,
    ])
  )
  const sessions = new Map(plan.sessions.map((s) => [s.id, s]))
  const updatePlate = useUpdatePlate()
  const [eaters, setEaters] = useState<number | null>(null)
  // Feedback is asked only once the plate's day has come.
  const [today] = useState(todayISO)

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-4 pb-6">
      {cell.badge && (
        <div>
          <BatchBadge {...cell.badge} />
        </div>
      )}

      <SessionWarnings plate={plate} sessions={plan.sessions} />
      <IssueList cell={cell} />
      {plate.reason && <WhyThis reason={plate.reason} />}

      <section
        className="flex flex-col gap-3"
        aria-labelledby="sheet-components"
      >
        <h3
          id="sheet-components"
          className="font-sans text-small font-semibold text-ink"
        >
          {t("plan.sheet.components")}
        </h3>
        <ul className="flex flex-col gap-4">
          {components.map((c) => {
            const session =
              c.cook_session_id != null
                ? sessions.get(c.cook_session_id)
                : undefined
            return (
              <li
                key={c.id}
                className="flex flex-col gap-2"
                data-testid="sheet-component"
              >
                <ComponentRow
                  plate={plate}
                  component={c}
                  readOnly={readOnly}
                  onPick={onPick}
                />
                {session && (
                  <BatchSection
                    session={session}
                    componentId={c.id}
                    colour={colours.get(session.id) ?? null}
                    slotName={slotName}
                    freezing={freezing}
                    readOnly={readOnly}
                    onOpenCell={onOpenCell}
                    onClose={onClose}
                  />
                )}
              </li>
            )
          })}
        </ul>
        {!readOnly && (
          <Button
            type="button"
            variant="outline"
            className="h-11 self-start md:pointer-fine:h-9"
            onClick={() => onPick({ kind: "add", plateId: plate.id })}
          >
            <Plus strokeWidth={1.5} aria-hidden />
            {t("plan.sheet.add_food")}
          </Button>
        )}
      </section>

      <div className="flex items-center justify-between gap-3">
        <span className="font-sans text-small font-semibold text-ink">
          {t("plan.sheet.eaters")}
        </span>
        <QuantityStepper
          label={t("plan.sheet.eaters")}
          value={eaters ?? plate.eaters}
          min={0.5}
          max={20}
          step={0.5}
          disabled={readOnly}
          testId="sheet-eaters"
          onChange={(next) => {
            setEaters(next)
            updatePlate.mutate(
              { id: plate.id, input: { eaters: next } },
              { onSettled: () => setEaters(null) }
            )
          }}
        />
      </div>

      <Macros components={components} />

      <NoteField key={plate.id} plate={plate} readOnly={readOnly} />

      {plate.date <= today && <FeedbackButtons plate={plate} />}

      {!readOnly && (
        <ReviewActions
          cell={cell}
          plan={plan}
          running={running}
          slotName={slotName}
          onClose={onClose}
        />
      )}

      {!readOnly && (
        <PlateActions
          plate={plate}
          cell={cell}
          onMarkNoPlate={onMarkNoPlate}
          onClose={onClose}
        />
      )}
    </div>
  )
}

/** The checker's issues on this cell (D22): `plan.issue.<code>` with its
 *  params, a generic line for codes the UI has no copy for. */
function IssueList({ cell }: { cell: BoardCell }) {
  const { t, i18n } = useTranslation()
  const issues = cell.warnings.flatMap((w) => (w.issue ? [w.issue] : []))
  if (issues.length === 0) return null
  return (
    <ul className="flex flex-col gap-2" data-testid="sheet-issues">
      {issues.map((issue, i) => (
        <li
          key={`${issue.code}-${i}`}
          className={cn(
            "flex items-start gap-2 rounded-cell border p-3 text-small text-ink",
            issue.severity === "violation" ? "border-warning/60" : "border-line"
          )}
        >
          <TriangleAlert
            className={cn(
              "mt-0.5 size-4 shrink-0",
              issue.severity === "violation" ? "text-warning" : "text-ink-muted"
            )}
            strokeWidth={2}
            aria-hidden
          />
          {(KNOWN_ISSUE_CODES as readonly string[]).includes(issue.code)
            ? t(`plan.issue.${issue.code}`, {
                ...issue.params,
                ...(typeof issue.params.last_eaten === "string" && {
                  last_eaten: formatDate(issue.params.last_eaten, i18n, {
                    day: "numeric",
                    month: "short",
                  }),
                }),
              })
            : t("plan.issue.other", { rule: issue.rule })}
        </li>
      ))}
    </ul>
  )
}

/** "Why this?": the chooser's one-line reason (spec §8.3). */
function WhyThis({ reason }: { reason: string }) {
  const { t } = useTranslation()
  return (
    <section aria-labelledby="sheet-why" className="flex flex-col gap-1">
      <h3
        id="sheet-why"
        className="font-sans text-small font-semibold text-ink"
      >
        {t("plan.review.why")}
      </h3>
      <p className="text-small text-ink" data-testid="sheet-why">
        {reason}
      </p>
    </section>
  )
}

/**
 * Review actions (D33, D34): Confirm (with Undo) or Unlock (asks first,
 * naming the batch closure), then Replace, Different side and Regenerate
 * day. Confirm closes the sheet (its Undo toast sits behind it);
 * regenerating does too: the board follows the run.
 */
function ReviewActions({
  cell,
  plan,
  running,
  slotName,
  onClose,
}: {
  cell: BoardCell & { plate: Plate }
  plan: Plan
  running: boolean
  slotName: (slotId: number) => string
  onClose: () => void
}) {
  const { t, i18n } = useTranslation()
  const confirm = useConfirm()
  const unlock = useUnlock()
  const regenerate = useRegenerate()
  const [unlocking, setUnlocking] = useState(false)
  const plate = cell.plate
  const draft = plate.state === "draft"
  const target = { kind: "plate", id: plate.id } as const
  const start = plan.window.start

  function regen(scope: RegenerateScope) {
    onClose()
    regenerate.mutate({ scope, start })
  }

  const alsoUnlocks = unlocking
    ? closureCells(plan, plate.id).map((c) =>
        t("plan.cell.name", {
          weekday: formatDate(c.date, i18n, { weekday: "long" }),
          slot: slotName(c.slotId),
        })
      )
    : []

  const button = "h-11 justify-start md:pointer-fine:h-9"
  return (
    <section
      className="flex flex-col gap-2 border-t border-line pt-4"
      data-testid="sheet-review"
    >
      <p className="flex items-center gap-1.5 text-small text-ink-muted">
        {!draft && (
          <Lock className="size-3 shrink-0" strokeWidth={2} aria-hidden />
        )}
        {t(draft ? "plan.review.draft_note" : "plan.review.confirmed_note")}
      </p>
      {draft ? (
        <Button
          type="button"
          className={button}
          onClick={() => {
            // Like every action with Undo, it closes the sheet: the toast
            // can't be reached behind it, and the board shows the padlocks.
            confirm.mutate(target)
            onClose()
          }}
        >
          <Check strokeWidth={1.5} aria-hidden />
          {t("plan.review.confirm")}
        </Button>
      ) : (
        <Button
          type="button"
          variant="outline"
          className={button}
          onClick={() => setUnlocking(true)}
        >
          <LockOpen strokeWidth={1.5} aria-hidden />
          {t("plan.review.unlock")}
        </Button>
      )}
      {draft && (
        <Button
          type="button"
          variant="outline"
          className={button}
          disabled={running}
          onClick={() => regen({ kind: "plate", plate_id: plate.id })}
        >
          <RefreshCw strokeWidth={1.5} aria-hidden />
          {t("plan.review.replace")}
        </Button>
      )}
      {draft && cell.sideComponentId !== null && (
        <Button
          type="button"
          variant="outline"
          className={button}
          disabled={running}
          onClick={() =>
            regen({ kind: "side", component_id: cell.sideComponentId! })
          }
        >
          <Shuffle strokeWidth={1.5} aria-hidden />
          {t("plan.review.different_side")}
        </Button>
      )}
      <Button
        type="button"
        variant="outline"
        className={button}
        disabled={running}
        onClick={() => regen({ kind: "day", date: plate.date })}
      >
        <CalendarSync strokeWidth={1.5} aria-hidden />
        {t("plan.review.regenerate_day")}
      </Button>
      <UnlockDialog
        open={unlocking}
        onOpenChange={setUnlocking}
        alsoUnlocks={alsoUnlocks}
        pending={unlock.isPending}
        onUnlock={() =>
          unlock.mutate(target, {
            onSuccess: (rows) => {
              setUnlocking(false)
              toast.success(
                t("plan.state.unlocked", { count: rows.plate_ids.length })
              )
            },
          })
        }
      />
    </section>
  )
}

function ComponentRow({
  plate,
  component: c,
  readOnly,
  onPick,
}: {
  plate: Plate
  component: PlateComponent
  readOnly: boolean
  onPick: (target: PickerTarget) => void
}) {
  const { t } = useTranslation()
  const update = useUpdateComponent()
  const remove = useRemoveComponent()
  const [portions, setPortions] = useState<number | null>(null)

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-1">
        <span className="min-w-0 flex-1 font-heading text-h3 text-ink">
          {c.food_name}
        </span>
        {!readOnly && (
          <>
            <Button
              type="button"
              variant="ghost"
              className="size-11 md:pointer-fine:size-8"
              aria-label={t("plan.sheet.swap", { name: c.food_name })}
              onClick={() =>
                onPick({
                  kind: "swap",
                  plateId: plate.id,
                  componentId: c.id,
                  name: c.food_name,
                })
              }
            >
              <ArrowLeftRight strokeWidth={1.5} aria-hidden />
            </Button>
            <Button
              type="button"
              variant="ghost"
              className="size-11 md:pointer-fine:size-8"
              aria-label={t("plan.sheet.remove", { name: c.food_name })}
              disabled={remove.isPending}
              onClick={() =>
                remove.mutate({ plateId: plate.id, componentId: c.id })
              }
            >
              <X strokeWidth={1.5} aria-hidden />
            </Button>
          </>
        )}
      </div>
      {c.kind === "composed" ? (
        <div className="flex items-center justify-between gap-3">
          <span className="text-small text-ink-muted">
            {t("plan.sheet.portions_per_eater")}
          </span>
          <QuantityStepper
            label={t("plan.sheet.portions_per_eater_of", { name: c.food_name })}
            value={portions ?? c.portions ?? 1}
            min={0.25}
            max={10}
            step={0.25}
            disabled={readOnly}
            testId="sheet-portions"
            onChange={(next) => {
              setPortions(next)
              update.mutate(
                {
                  plateId: plate.id,
                  componentId: c.id,
                  input: { portions: next },
                },
                { onSettled: () => setPortions(null) }
              )
            }}
          />
        </div>
      ) : (
        <LeafQuantity
          // A saved amount remounts the draft from the server's value.
          key={`${c.amount}|${c.unit}`}
          plate={plate}
          component={c}
          readOnly={readOnly}
        />
      )}
    </div>
  )
}

/** A leaf's amount and unit per eater: edited as a draft, then saved. */
function LeafQuantity({
  plate,
  component: c,
  readOnly,
}: {
  plate: Plate
  component: PlateComponent
  readOnly: boolean
}) {
  const { t, i18n } = useTranslation()
  const food = useFood(c.food_id)
  const update = useUpdateComponent()
  const saved: QuantityUnitValue = {
    amount: c.amount ?? 1,
    unit: c.unit ?? "g",
  }
  const [draft, setDraft] = useState(saved)
  const dirty = draft.amount !== saved.amount || draft.unit !== saved.unit

  if (readOnly || !food.data) {
    return (
      <span className="text-small text-ink-muted tabular-nums">
        {formatAmount(saved.amount, i18n)}{" "}
        {t(`units.${saved.unit}`, { defaultValue: saved.unit })}
      </span>
    )
  }
  return (
    <div className="flex flex-col gap-2">
      <QuantityUnitInput food={food.data} value={draft} onChange={setDraft} />
      {dirty && (
        <Button
          type="button"
          className="h-11 self-start md:pointer-fine:h-9"
          disabled={update.isPending}
          onClick={() =>
            update.mutate({
              plateId: plate.id,
              componentId: c.id,
              input: { amount: draft.amount, unit: draft.unit },
            })
          }
        >
          {t("plan.sheet.update_amount")}
        </Button>
      )}
    </div>
  )
}

/** Inline warnings for this plate's batches, with "Cook N more" for a
 *  deficit (D13). */
function SessionWarnings({
  plate,
  sessions,
}: {
  plate: Plate
  sessions: Session[]
}) {
  const { t, i18n } = useTranslation()
  const update = useUpdateSession()
  const items: { session: Session; warning: SessionWarning }[] = []
  for (const s of sessions) {
    const onPlate = s.links.filter((l) => l.plate_id === plate.id)
    if (onPlate.length === 0) continue
    const cooksHere = onPlate.some((l) => l.kind === "fresh")
    for (const w of s.warnings) {
      if (w.plate_id === plate.id || (w.plate_id === null && cooksHere)) {
        items.push({ session: s, warning: w })
      }
    }
  }
  if (items.length === 0) return null

  return (
    <ul className="flex flex-col gap-2" data-testid="sheet-warnings">
      {items.map(({ session, warning }) => {
        const short = session.consumed - session.portions
        return (
          <li
            key={`${session.id}-${warning.code}-${warning.plate_id}`}
            className="flex flex-col gap-2 rounded-cell border border-warning/60 p-3"
          >
            <span className="flex items-start gap-2 text-small text-ink">
              <TriangleAlert
                className="mt-0.5 size-4 shrink-0 text-warning"
                strokeWidth={2}
                aria-hidden
              />
              {t(`plan.warning.${warning.code}`, {
                name: session.food_name,
                count: short,
                amount: formatAmount(short, i18n),
              })}
            </span>
            {warning.code === "deficit" && short > 0 && (
              <Button
                type="button"
                variant="outline"
                className="h-11 self-start md:pointer-fine:h-9"
                disabled={update.isPending}
                onClick={() =>
                  update.mutate({
                    id: session.id,
                    input: {
                      portions: session.consumed,
                      frozen_portions: Math.min(
                        session.frozen_portions,
                        session.consumed
                      ),
                      note: session.note,
                    },
                  })
                }
              >
                {t("plan.warning.cook_more", {
                  count: formatAmount(short, i18n),
                })}
              </Button>
            )}
          </li>
        )
      })}
    </ul>
  )
}

/** kcal, protein, fat and carbs for one eater: composed foods per portion
 *  × portions, leaves per 100 g × grams. Eaters do not change it. */
function Macros({ components }: { components: PlateComponent[] }) {
  const { t, i18n } = useTranslation()
  const ids = [...new Set(components.map((c) => c.food_id))]
  const macros = useFoodMacros(ids)
  if (!macros.data) return null
  const total = plateMacros(components, macros.data.foods)
  const g = t("units.g")
  return (
    <section aria-labelledby="sheet-macros" className="flex flex-col gap-1">
      <h3
        id="sheet-macros"
        className="font-sans text-small font-semibold text-ink"
      >
        {t("plan.sheet.macros")}
      </h3>
      <p className="text-body text-ink tabular-nums" data-testid="sheet-kcal">
        {formatNumber(total.kcal, i18n)} {t("units.kcal")}
      </p>
      <p className="text-small text-ink-muted tabular-nums">
        {t("plan.sheet.macro_line", {
          protein: `${formatNumber(total.protein, i18n)} ${g}`,
          fat: `${formatNumber(total.fat, i18n)} ${g}`,
          carbs: `${formatNumber(total.carbs, i18n)} ${g}`,
        })}
      </p>
    </section>
  )
}

/** The plate note, saved on blur and, if still unsaved, when the sheet
 *  closes: Escape unmounts the sheet without a blur. */
function NoteField({ plate, readOnly }: { plate: Plate; readOnly: boolean }) {
  const { t } = useTranslation()
  const { mutate } = useUpdatePlate()
  const [draft, setDraft] = useState(plate.note ?? "")
  // What the server holds, or the last value sent: never send it twice.
  const saved = useRef(plate.note)
  const latest = useRef({ draft, readOnly })
  useEffect(() => {
    latest.current = { draft, readOnly }
  })

  const save = useCallback(
    (text: string) => {
      const note = text.trim() || null
      if (note === saved.current) return
      saved.current = note
      mutate({ id: plate.id, input: { note } })
    },
    [mutate, plate.id]
  )

  useEffect(
    () => () => {
      if (!latest.current.readOnly) save(latest.current.draft)
    },
    [save]
  )

  return (
    <label className="flex flex-col gap-1">
      <span className="font-sans text-small font-semibold text-ink">
        {t("plan.sheet.note")}
      </span>
      <Textarea
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        readOnly={readOnly}
        rows={2}
        placeholder={t("plan.sheet.note_placeholder")}
        data-testid="sheet-note"
        onBlur={() => save(draft)}
      />
    </label>
  )
}

function FeedbackButtons({ plate }: { plate: Plate }) {
  const { t } = useTranslation()
  const record = useRecordFeedback()
  const clear = useClearFeedback()
  const current = plate.feedback?.status ?? null
  const busy = record.isPending || clear.isPending
  return (
    <section aria-labelledby="sheet-feedback" className="flex flex-col gap-2">
      <h3
        id="sheet-feedback"
        className="font-sans text-small font-semibold text-ink"
      >
        {t("plan.sheet.feedback")}
      </h3>
      <div className="flex flex-wrap gap-2">
        {FEEDBACK.map((status) => {
          const active = current === status
          return (
            <button
              key={status}
              type="button"
              aria-pressed={active}
              disabled={busy}
              data-testid={`feedback-${status}`}
              onClick={() =>
                active
                  ? clear.mutate(plate.id)
                  : record.mutate({ plateId: plate.id, input: { status } })
              }
              className={cn(
                "inline-flex min-h-11 items-center rounded-full border px-4 text-small transition-colors duration-150 ease-out focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-panel focus-visible:outline-none disabled:opacity-60 md:pointer-fine:min-h-9",
                active
                  ? "border-accent bg-accent text-on-accent"
                  : "border-line text-ink hover:bg-panel-sunken"
              )}
            >
              {t(`plan.feedback.${status}`)}
            </button>
          )
        })}
      </div>
    </section>
  )
}

function PlateActions({
  plate,
  cell,
  onMarkNoPlate,
  onClose,
}: {
  plate: Plate
  cell: BoardCell
  onMarkNoPlate: (cell: BoardCell) => void
  onClose: () => void
}) {
  const { t } = useTranslation()
  const deletePlate = useDeletePlate()
  const createPreset = useCreatePreset()
  const [naming, setNaming] = useState(false)
  const [name, setName] = useState(cell.mainName ?? "")

  function saveRoutine(e: React.FormEvent) {
    e.preventDefault()
    const trimmed = name.trim()
    if (!trimmed) return
    createPreset.mutate(
      { name: trimmed, plate_ids: [plate.id] },
      {
        onSuccess: () => {
          setNaming(false)
          toast.success(t("plan.sheet.routine_saved", { name: trimmed }))
        },
      }
    )
  }

  return (
    <section className="flex flex-col gap-2 border-t border-line pt-4">
      {naming ? (
        <form className="flex flex-col gap-2" onSubmit={saveRoutine}>
          <label className="flex flex-col gap-1">
            <span className="text-small text-ink">
              {t("plan.sheet.routine_name")}
            </span>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="h-11 md:pointer-fine:h-9"
              autoFocus
              data-testid="sheet-routine-name"
            />
          </label>
          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              className="h-11 md:pointer-fine:h-9"
              onClick={() => setNaming(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              className="h-11 flex-1 md:pointer-fine:h-9"
              disabled={!name.trim() || createPreset.isPending}
            >
              {t("plan.sheet.save_routine")}
            </Button>
          </div>
        </form>
      ) : (
        <Button
          type="button"
          variant="outline"
          className="h-11 justify-start md:pointer-fine:h-9"
          onClick={() => setNaming(true)}
        >
          {t("plan.sheet.save_routine")}
        </Button>
      )}
      <Button
        type="button"
        variant="outline"
        className="h-11 justify-start md:pointer-fine:h-9"
        onClick={() => {
          onClose()
          onMarkNoPlate(cell)
        }}
      >
        {t("plan.sheet.mark_no_plate")}
      </Button>
      <Button
        type="button"
        variant="outline"
        className="h-11 justify-start text-danger md:pointer-fine:h-9"
        onClick={() => {
          deletePlate.mutate(plate.id)
          onClose()
        }}
      >
        {t("plan.sheet.delete")}
      </Button>
    </section>
  )
}
