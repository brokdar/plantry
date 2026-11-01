import { X } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import type { Plan, PlanCell, ReplanCell } from "@/lib/api/plan"
import { formatAmount, formatDate } from "@/lib/plan/format"
import { useReplan, useReplanPreview } from "@/lib/queries/plan"
import { useSettingItems } from "@/lib/queries/settings"
import { slotLabel } from "@/lib/slot-label"
import { toast } from "@/lib/toast"
import { cn } from "@/lib/utils"

import { useIsPhone } from "./use-is-phone"

interface ReplanSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  plan: Plan
  /** The first day the re-plan touches (≥ today, inside the window). */
  from: string
  /** Cells already asked for (Mark as No plate on a confirmed plate). */
  initialCells: ReplanCell[]
}

/**
 * The mid-week re-plan (UX §5.10, AC-5): the user ticks the meals that are
 * off from `from` to the window's end; the sheet previews what that changes
 * ("Thu and Fri are now No plate", spare batch portions with Freeze or Cook
 * less when the recipe freezes) and offers "Re-plan the rest of the week".
 * Confirmed plates among them are listed under "These were already bought
 * for" with Change them / Keep them (D34: purchase-affecting, so it asks).
 * A started run shows as the board's progress and draft overlay. No Undo:
 * the removed snapshot can't restore into the new No-plate cells.
 */
export function ReplanSheet({
  open,
  onOpenChange,
  plan,
  from,
  initialCells,
}: ReplanSheetProps) {
  const phone = useIsPhone()
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side={phone ? "bottom" : "right"}
        className={cn(
          "flex flex-col gap-0 bg-panel p-0 text-ink shadow-sheet",
          phone
            ? "max-h-[90dvh] rounded-t-sheet"
            : "w-full sm:max-w-md md:max-w-md"
        )}
        data-testid="replan-sheet"
        showCloseButton={false}
      >
        <ReplanBody
          plan={plan}
          from={from}
          initialCells={initialCells}
          onClose={() => onOpenChange(false)}
        />
      </SheetContent>
    </Sheet>
  )
}

const cellKey = (c: ReplanCell) => `${c.date}:${c.slot_id}`

function ReplanBody({
  plan,
  from,
  initialCells,
  onClose,
}: Omit<ReplanSheetProps, "open" | "onOpenChange"> & { onClose: () => void }) {
  const { t, i18n } = useTranslation()
  const settings = useSettingItems()
  const replan = useReplan()
  const [selected, setSelected] = useState<ReplanCell[]>(initialCells)
  const [regenerate, setRegenerate] = useState(false)
  const [spareMode, setSpareMode] = useState<"freeze" | "keep">("freeze")
  const to = plan.window.end
  const preview = useReplanPreview(from, to, selected)

  const slotOrder = [...plan.slots].sort((a, b) => a.sort_order - b.sort_order)
  const slotName = (id: number) =>
    slotLabel(t, plan.slots.find((s) => s.id === id)?.name_key ?? "")
  const weekday = (date: string) => formatDate(date, i18n, { weekday: "long" })
  const cellName = (c: ReplanCell) =>
    t("plan.cell.name", { weekday: weekday(c.date), slot: slotName(c.slot_id) })

  const days = [...new Set(plan.cells.map((c) => c.date))]
    .filter((d) => d >= from && d <= to)
    .sort()
    .map((date) => ({
      date,
      cells: slotOrder.flatMap((s) => {
        const c = plan.cells.find((x) => x.date === date && x.slot_id === s.id)
        return c ? [c] : []
      }),
    }))
  const isSelected = (c: ReplanCell) =>
    selected.some((s) => cellKey(s) === cellKey(c))
  const open = (cells: PlanCell[]) => cells.filter((c) => c.mode !== "none")

  function toggle(c: PlanCell) {
    setSelected((prev) =>
      isSelected(c)
        ? prev.filter((s) => cellKey(s) !== cellKey(c))
        : [...prev, { date: c.date, slot_id: c.slot_id }]
    )
  }

  function toggleDay(cells: PlanCell[]) {
    const day = open(cells)
    const all = day.every(isSelected)
    setSelected((prev) =>
      all
        ? prev.filter((s) => !day.some((c) => cellKey(c) === cellKey(s)))
        : [
            ...prev,
            ...day
              .filter((c) => !isSelected(c))
              .map((c) => ({ date: c.date, slot_id: c.slot_id })),
          ]
    )
  }

  // "Thursday and Friday": whole days by name, else the single meals.
  const list = new Intl.ListFormat(i18n.language, { type: "conjunction" })
  const changed = days.flatMap(({ date, cells }) => {
    const day = open(cells)
    const picked = day.filter(isSelected)
    if (picked.length === 0) return []
    if (picked.length === day.length) return [weekday(date)]
    return picked.map(cellName)
  })

  const data = preview.data
  const freezable = (data?.spare ?? []).filter((s) => s.freezable)
  const freezePortions = freezable.reduce((n, s) => n + s.portions, 0)
  const confirmed = data?.confirmed_affected ?? []
  const assistant = Boolean(settings.data?.["ai.provider"]?.value)
  // A preview for an older choice may miss confirmed plates of the new one.
  const settled =
    selected.length === 0 || (!!data && !preview.isPlaceholderData)
  const busy = replan.isPending || !settled

  function submit(unlock: boolean) {
    replan.mutate(
      {
        from,
        to,
        none_cells: selected,
        unlock_confirmed: unlock,
        spare: freezable.length > 0 ? spareMode : "keep",
        regenerate,
        // No provider configured: the heuristic fills (spec §13 "Without
        // AI"); with one, the server's default chooser (D11).
        ...(assistant ? {} : { chooser: "heuristic" as const }),
      },
      {
        onSuccess: ({ removed }) => {
          const count = removed.plates.length
          toast.success(
            count > 0
              ? t("plan.replan.done_removed", { count })
              : t("plan.replan.done")
          )
          onClose()
        },
      }
    )
  }

  const chip =
    "inline-flex min-h-11 cursor-pointer flex-col items-start justify-center rounded-lg border px-3 py-1 text-small transition-colors duration-150 ease-out has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-accent has-[:disabled]:cursor-default md:pointer-fine:min-h-9"
  const chipOn = "border-accent bg-accent text-on-accent"
  const chipOff = "border-line bg-panel text-ink hover:border-ink-muted"
  const action = "h-11 md:pointer-fine:h-9"

  return (
    <>
      <SheetClose asChild>
        <Button
          type="button"
          variant="ghost"
          className="absolute top-3 right-3 size-11 md:pointer-fine:size-8"
          aria-label={t("common.close")}
        >
          <X strokeWidth={1.5} aria-hidden />
        </Button>
      </SheetClose>
      <SheetHeader className="px-4 pt-5 pr-14 pb-3">
        <SheetTitle className="font-heading text-h2 font-normal text-ink">
          {from === plan.window.today
            ? t("plan.replan.title_today")
            : t("plan.replan.title_from", { day: weekday(from) })}
        </SheetTitle>
        <SheetDescription className="text-small text-ink-muted">
          {t("plan.replan.intro")}
        </SheetDescription>
      </SheetHeader>

      <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-4 pb-4">
        <fieldset className="flex flex-col gap-3">
          <legend className="mb-1 text-small font-semibold text-ink">
            {t("plan.replan.cells_legend")}
          </legend>
          {days.map(({ date, cells }) => {
            const day = open(cells)
            const whole = day.length > 0 && day.every(isSelected)
            const headingId = `replan-day-${date}`
            return (
              <div
                key={date}
                role="group"
                aria-labelledby={headingId}
                className="flex flex-col gap-1.5"
                data-testid="replan-day"
              >
                <p id={headingId} className="text-small text-ink-muted">
                  {formatDate(date, i18n, {
                    weekday: "long",
                    day: "numeric",
                    month: "short",
                  })}
                </p>
                <div className="flex flex-wrap gap-2">
                  {cells.map((c) => {
                    const none = c.mode === "none"
                    const on = none || isSelected(c)
                    const food = c.plate?.components[0]?.food_name
                    return (
                      <label
                        key={cellKey(c)}
                        className={cn(chip, on ? chipOn : chipOff)}
                      >
                        <input
                          type="checkbox"
                          checked={on}
                          disabled={none}
                          onChange={() => toggle(c)}
                          aria-label={cellName(c)}
                          className="sr-only"
                        />
                        <span>{slotName(c.slot_id)}</span>
                        {(none || food) && (
                          <span
                            className={cn(
                              "text-caption",
                              on ? "text-on-accent" : "text-ink-muted"
                            )}
                          >
                            {none ? t("plan.cell.no_plate") : food}
                          </span>
                        )}
                      </label>
                    )
                  })}
                  {day.length > 1 && (
                    <label className={cn(chip, whole ? chipOn : chipOff)}>
                      <input
                        type="checkbox"
                        checked={whole}
                        onChange={() => toggleDay(cells)}
                        className="sr-only"
                      />
                      {t("plan.replan.whole_day")}
                    </label>
                  )}
                </div>
              </div>
            )
          })}
        </fieldset>

        {(changed.length > 0 || (data?.spare.length ?? 0) > 0) && (
          <section
            className="flex flex-col gap-1 border-t border-line pt-4 text-body text-ink"
            data-testid="replan-changes"
            aria-live="polite"
          >
            {changed.length > 0 && (
              <p>
                {t("plan.replan.now_none", {
                  count: changed.length,
                  cells: list.format(changed),
                })}
              </p>
            )}
            {data?.spare.map((s) => (
              <p key={s.session_id}>
                {t(
                  s.freezable ? "plan.replan.spare" : "plan.replan.unassigned",
                  {
                    count: s.portions,
                    amount: formatAmount(s.portions, i18n),
                    name: s.food_name,
                  }
                )}
              </p>
            ))}
          </section>
        )}

        {freezable.length > 0 && (
          <fieldset className="flex flex-col gap-1">
            <legend className="mb-1 text-small font-semibold text-ink">
              {t("plan.replan.spare_legend")}
            </legend>
            {(["freeze", "keep"] as const).map((mode) => (
              <label
                key={mode}
                className="flex min-h-11 cursor-pointer items-center gap-3 text-body text-ink md:pointer-fine:min-h-9"
              >
                <input
                  type="radio"
                  name="replan-spare"
                  checked={spareMode === mode}
                  onChange={() => setSpareMode(mode)}
                  className="size-4 accent-accent"
                />
                {t(
                  mode === "freeze"
                    ? "plan.replan.spare_freeze"
                    : "plan.replan.spare_keep",
                  {
                    count: freezePortions,
                    amount: formatAmount(freezePortions, i18n),
                  }
                )}
              </label>
            ))}
          </fieldset>
        )}

        <label className="flex cursor-pointer items-start gap-3 border-t border-line pt-4">
          <input
            type="checkbox"
            checked={regenerate}
            onChange={(e) => setRegenerate(e.target.checked)}
            aria-label={t("plan.replan.regenerate")}
            aria-describedby="replan-regenerate-hint"
            className="mt-1 size-4 accent-accent"
          />
          <span className="flex flex-col">
            <span className="text-body text-ink" aria-hidden>
              {t("plan.replan.regenerate")}
            </span>
            <span
              id="replan-regenerate-hint"
              className="text-small text-ink-muted"
            >
              {t("plan.replan.regenerate_hint", { day: weekday(from) })}
            </span>
          </span>
        </label>

        {confirmed.length > 0 && (
          <section
            className="flex flex-col gap-1 rounded-cell border border-warning/40 bg-panel-sunken p-3"
            data-testid="replan-bought"
          >
            <h3 className="font-sans text-small font-semibold text-ink">
              {t("plan.replan.bought_title")}
            </h3>
            <ul className="flex flex-col gap-0.5 text-small text-ink">
              {confirmed.map((p) => (
                <li key={p.plate_id}>
                  {t("plan.replan.bought_item", {
                    cell: cellName(p),
                    name: p.food_name,
                  })}
                </li>
              ))}
            </ul>
          </section>
        )}
      </div>

      <SheetFooter className="flex-row justify-end gap-2 border-t border-line px-4 py-3">
        {confirmed.length > 0 ? (
          <>
            <Button
              type="button"
              variant="outline"
              className={action}
              onClick={onClose}
            >
              {t("plan.replan.keep")}
            </Button>
            <Button
              type="button"
              className={action}
              disabled={busy}
              onClick={() => submit(true)}
            >
              {t("plan.replan.change")}
            </Button>
          </>
        ) : (
          <>
            <Button
              type="button"
              variant="outline"
              className={action}
              onClick={onClose}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="button"
              className={action}
              disabled={busy || (selected.length === 0 && !regenerate)}
              onClick={() => submit(false)}
            >
              {t("plan.replan.apply")}
            </Button>
          </>
        )}
      </SheetFooter>
    </>
  )
}
