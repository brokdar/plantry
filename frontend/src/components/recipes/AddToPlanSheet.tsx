import { useState } from "react"
import { Link, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { X } from "lucide-react"

import { QuantityStepper } from "@/components/plan/QuantityStepper"
import { useIsPhone } from "@/components/plan/use-is-phone"
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
import { Skeleton } from "@/components/ui/skeleton"
import type { ComposedFood } from "@/lib/api/foods"
import type { PlanWindow } from "@/lib/api/plan"
import { formatDate } from "@/lib/plan/format"
import { usePlan } from "@/lib/queries/plan"
import { useCreatePlate, useReplaceDraftPlate } from "@/lib/queries/plates"
import { useProfile } from "@/lib/queries/profile"
import { openCells, type OpenCell } from "@/lib/recipes/add-to-plan"
import { slotLabel } from "@/lib/slot-label"
import { toast } from "@/lib/toast"
import { cn } from "@/lib/utils"

type AddToPlanSheetProps = {
  recipe: Pick<ComposedFood, "id" | "name">
  open: boolean
  onOpenChange: (open: boolean) => void
}

/** Add to plan from recipe detail (D36, UX §5.8): the free `plan` cells of
 *  the current and next window, and those holding only a draft plate
 *  (marked as replacing it), the first free one preselected, and an eaters
 *  stepper. Add creates a plate with the recipe, cooked fresh, deleting the
 *  cell's draft first. */
export function AddToPlanSheet({
  recipe,
  open,
  onOpenChange,
}: AddToPlanSheetProps) {
  const { t } = useTranslation()
  const phone = useIsPhone()
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side={phone ? "bottom" : "right"}
        showCloseButton={false}
        className={cn(
          "flex flex-col gap-0 bg-panel p-0 text-ink shadow-sheet",
          phone
            ? "max-h-[90dvh] rounded-t-sheet"
            : "w-full sm:max-w-md md:max-w-md"
        )}
        data-testid="add-to-plan-sheet"
      >
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
        <SheetHeader className="px-4 pt-5 pr-14 pb-3 md:px-6">
          <SheetTitle className="font-heading text-h2 font-normal text-ink">
            {t("add_to_plan.title")}
          </SheetTitle>
          <SheetDescription className="text-small text-ink-muted">
            {t("add_to_plan.description", { name: recipe.name })}
          </SheetDescription>
        </SheetHeader>
        <AddToPlanBody recipe={recipe} onDone={() => onOpenChange(false)} />
      </SheetContent>
    </Sheet>
  )
}

/** The window that pairs with the anchored one to make "the current and
 *  the next": the previous one when the anchored window has not started
 *  yet, else the next one. */
function otherWindow(w: PlanWindow): string {
  return w.today < w.start ? w.prev_start : w.next_start
}

/** Mounted only while the sheet is open, so the plan loads on demand. */
function AddToPlanBody({
  recipe,
  onDone,
}: {
  recipe: AddToPlanSheetProps["recipe"]
  onDone: () => void
}) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  // The anchored window is today's or, with a "next" anchor, the one after.
  const anchored = usePlan()
  const otherStart = anchored.data && otherWindow(anchored.data.window)
  // Until the anchored window is in, this shares its query.
  const other = usePlan(otherStart)
  const profile = useProfile()
  const createPlate = useCreatePlate()
  const replaceDraft = useReplaceDraftPlate()
  const [picked, setPicked] = useState<string | null>(null)
  const [eaters, setEaters] = useState<number | null>(null)

  const loading = anchored.isPending || (!!otherStart && other.isPending)
  const failed = anchored.isError || other.isError
  const days =
    anchored.data && other.data && otherStart
      ? openCells([anchored.data, other.data])
      : []
  const cells = days.flatMap((d) => d.cells)
  const selected =
    cells.find((c) => c.key === picked) ??
    cells.find((c) => !c.replaces) ??
    cells[0]
  const shownEaters = eaters ?? profile.data?.household_portions ?? 1

  const meal = (cell: OpenCell) => slotLabel(t, cell.slot.name_key)
  const day = (date: string) =>
    formatDate(date, i18n, { weekday: "long", day: "numeric", month: "long" })

  const replacesText = (cell: OpenCell) =>
    cell.replaces &&
    (cell.replaces.name
      ? t("add_to_plan.replaces", { name: cell.replaces.name })
      : t("add_to_plan.replaces_unnamed"))

  function add() {
    if (!selected) return
    const input = {
      date: selected.date,
      slot_id: selected.slot.id,
      // Until the profile is in, the server's household default applies.
      eaters: eaters ?? profile.data?.household_portions,
      components: [{ food_id: recipe.id, portions: 1 }],
    }
    const options = {
      onSuccess: () => {
        onDone()
        toast.success(
          t("add_to_plan.added", {
            name: recipe.name,
            day: day(selected.date),
            meal: meal(selected),
          }),
          {
            action: {
              label: t("add_to_plan.open_plan"),
              onClick: () =>
                void navigate({
                  to: "/plan",
                  search: { start: selected.windowStart, day: selected.date },
                }),
            },
          }
        )
      },
    }
    if (selected.replaces)
      replaceDraft.mutate(
        { draftId: selected.replaces.plateId, input },
        options
      )
    else createPlate.mutate(input, options)
  }

  if (failed) {
    return (
      <div
        role="alert"
        className="flex flex-col items-start gap-3 px-4 md:px-6"
      >
        <p className="text-body text-ink">{t("add_to_plan.load_error")}</p>
        <Button
          type="button"
          variant="outline"
          className="min-h-11 md:pointer-fine:min-h-8"
          onClick={() => {
            void anchored.refetch()
            if (otherStart) void other.refetch()
          }}
        >
          {t("common.retry")}
        </Button>
      </div>
    )
  }

  if (loading) {
    return (
      <div className="flex flex-col gap-3 px-4 md:px-6">
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-11 w-full rounded-cell" />
        ))}
      </div>
    )
  }

  if (cells.length === 0) {
    return (
      <div
        className="flex flex-col items-start gap-2 px-4 pb-6 md:px-6"
        data-testid="add-to-plan-empty"
      >
        <h3 className="text-h3 font-semibold text-ink">
          {t("add_to_plan.empty_title")}
        </h3>
        <p className="text-body text-ink-muted">
          {t("add_to_plan.empty_body")}
        </p>
        <Button asChild variant="outline" className="mt-2 min-h-11">
          <Link to="/plan">{t("add_to_plan.open_plan")}</Link>
        </Button>
      </div>
    )
  }

  return (
    <>
      <fieldset className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-4 pb-4 md:px-6">
        <legend className="mb-2 text-small font-semibold text-ink">
          {t("add_to_plan.meals")}
        </legend>
        {days.map((d) => (
          <div key={d.date} className="flex flex-col gap-1.5">
            <p className="text-small text-ink-muted first-letter:uppercase">
              {day(d.date)}
            </p>
            <div className="flex flex-wrap gap-2">
              {d.cells.map((cell) => {
                const checked = cell.key === selected?.key
                const replaces = replacesText(cell)
                return (
                  <label
                    key={cell.key}
                    className={cn(
                      "inline-flex min-h-11 cursor-pointer flex-col items-start justify-center rounded-lg border px-3 py-1 text-small transition-colors duration-150 ease-out has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-accent md:pointer-fine:min-h-8",
                      checked
                        ? "border-accent bg-accent font-semibold text-on-accent"
                        : "border-line bg-panel text-ink hover:border-ink-muted",
                      replaces && !checked && "border-dashed"
                    )}
                  >
                    <input
                      type="radio"
                      name="add-to-plan-cell"
                      value={cell.key}
                      checked={checked}
                      onChange={() => setPicked(cell.key)}
                      aria-label={
                        replaces
                          ? t("add_to_plan.cell_replaces", {
                              day: day(cell.date),
                              meal: meal(cell),
                              replaces,
                            })
                          : t("add_to_plan.cell", {
                              day: day(cell.date),
                              meal: meal(cell),
                            })
                      }
                      className="sr-only"
                      data-testid={`add-to-plan-cell-${cell.key}`}
                    />
                    <span>{meal(cell)}</span>
                    {replaces && (
                      <span
                        className={cn(
                          "text-caption font-normal",
                          checked ? "text-on-accent" : "text-ink-muted"
                        )}
                      >
                        {replaces}
                      </span>
                    )}
                  </label>
                )
              })}
            </div>
          </div>
        ))}
      </fieldset>
      <SheetFooter className="flex-row items-center justify-between gap-3 border-t border-line px-4 py-3 md:px-6">
        <div className="flex items-center gap-3">
          <span className="text-small text-ink-muted">
            {t("plan.sheet.eaters")}
          </span>
          <QuantityStepper
            label={t("plan.sheet.eaters")}
            value={shownEaters}
            min={0.5}
            max={20}
            step={0.5}
            onChange={setEaters}
            testId="add-to-plan-eaters"
          />
        </div>
        <Button
          type="button"
          className="min-h-11 md:pointer-fine:min-h-9"
          disabled={createPlate.isPending || replaceDraft.isPending}
          onClick={add}
          data-testid="add-to-plan-submit"
        >
          {t("add_to_plan.add")}
        </Button>
      </SheetFooter>
    </>
  )
}
