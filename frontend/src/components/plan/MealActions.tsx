import { useRef, useState, type ReactNode } from "react"
import { useTranslation } from "react-i18next"

import { MoreHorizontal } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import type { AlsoChanged } from "@/lib/api/meal"
import type { Plan } from "@/lib/api/plan"
import {
  alsoChangedLines,
  cellLabel,
  type CellOption,
} from "@/lib/plan/meal-actions"
import type { DescribeAlsoChanged } from "@/lib/queries/meal"
import { toastError } from "@/lib/toast"

const button = "h-11 md:pointer-fine:h-9"

/**
 * The also-changed preview (meal editing 2a): `confirm(preview, commit)`
 * runs the dry run; when it changes nothing else, `commit` runs at once,
 * else `dialog` lists the other meals in plain words with Confirm and
 * Cancel. `describe` words the same list for the undo toast.
 */
export function useAlsoChangedConfirm(
  plan: Plan,
  slotName: (id: number) => string
): {
  confirm: (preview: () => Promise<AlsoChanged[]>, commit: () => void) => void
  describe: DescribeAlsoChanged
  dialog: ReactNode
} {
  const { t, i18n } = useTranslation()
  const [pending, setPending] = useState<{
    lines: string[]
    commit: () => void
  } | null>(null)
  const lines = (list: AlsoChanged[]) =>
    alsoChangedLines(list, plan, t, i18n, slotName)

  // A second call while a preview is in flight or the dialog is open is
  // ignored (a double click or a second drop must not stack another action).
  const previewing = useRef(false)

  function confirm(preview: () => Promise<AlsoChanged[]>, commit: () => void) {
    if (previewing.current || pending) return
    previewing.current = true
    preview().then(
      (list) => {
        previewing.current = false
        if (list.length === 0) commit()
        else setPending({ lines: lines(list), commit })
      },
      (err) => {
        previewing.current = false
        toastError(err, t)
      }
    )
  }

  const dialog = (
    <Dialog
      open={pending !== null}
      onOpenChange={(o) => !o && setPending(null)}
    >
      <DialogContent showCloseButton={false} data-testid="also-changed">
        <DialogHeader>
          <DialogTitle>{t("plan.meal.also.title")}</DialogTitle>
          <DialogDescription>{t("plan.meal.also.body")}</DialogDescription>
        </DialogHeader>
        <ul className="flex flex-col gap-1.5 text-small text-ink">
          {pending?.lines.map((l, i) => (
            <li key={i}>{l}</li>
          ))}
        </ul>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            className={button}
            onClick={() => setPending(null)}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            className={button}
            onClick={() => {
              pending?.commit()
              setPending(null)
            }}
          >
            {t("common.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )

  return {
    confirm,
    describe: (list) => (list.length ? lines(list).join(" · ") : undefined),
    dialog,
  }
}

export type DayAction = "away" | "plan" | "regenerate"

/** The day header's menu: Away all day, Plan this day, Regenerate day. */
export function DayMenu({
  label,
  onAction,
}: {
  label: string
  onAction: (action: DayAction) => void
}) {
  const { t } = useTranslation()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          className="size-11 text-ink-muted md:pointer-fine:size-7"
          aria-label={t("plan.meal.day_menu", { day: label })}
          data-testid="day-menu"
        >
          <MoreHorizontal strokeWidth={1.5} aria-hidden />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuGroup>
          <DropdownMenuItem onSelect={() => onAction("away")}>
            {t("plan.meal.away_day")}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => onAction("plan")}>
            {t("plan.meal.plan_day")}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => onAction("regenerate")}>
            {t("plan.review.regenerate_day")}
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

/** A list of board cells to choose one from (Move to…, Copy to…, Cook on
 *  another day). Each is a button, so Tab and Enter work. */
export function CellList({
  cells,
  slotName,
  onChoose,
  testId = "cell-list",
}: {
  cells: CellOption[]
  slotName: (id: number) => string
  onChoose: (cell: CellOption) => void
  testId?: string
}) {
  const { t, i18n } = useTranslation()
  if (cells.length === 0)
    return (
      <p className="text-small text-ink-muted">{t("plan.meal.no_cells")}</p>
    )
  return (
    <ul
      className="flex max-h-[50vh] flex-col gap-1 overflow-y-auto"
      data-testid={testId}
    >
      {cells.map((c) => (
        <li key={c.key}>
          <Button
            type="button"
            variant="ghost"
            className="h-11 w-full justify-between gap-3 md:pointer-fine:h-9"
            onClick={() => onChoose(c)}
          >
            <span>{cellLabel(c, t, i18n, slotName)}</span>
            {c.name && (
              <span className="truncate text-small text-ink-muted">
                {c.name}
              </span>
            )}
          </Button>
        </li>
      ))}
    </ul>
  )
}

/**
 * Move to… / Copy to… from the plate sheet: a cell list; a taken cell asks
 * Swap or Replace (Move only).
 */
export function CellChoiceDialog({
  open,
  title,
  cells,
  slotName,
  onOpenChange,
  onChoose,
}: {
  open: boolean
  title: string
  cells: CellOption[]
  slotName: (id: number) => string
  onOpenChange: (open: boolean) => void
  onChoose: (cell: CellOption, occupant?: "swap" | "drop") => void
}) {
  const { t, i18n } = useTranslation()
  const [taken, setTaken] = useState<CellOption | null>(null)
  const close = (o: boolean) => {
    if (!o) setTaken(null)
    onOpenChange(o)
  }
  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent data-testid="cell-choice">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription className="sr-only">{title}</DialogDescription>
        </DialogHeader>
        {taken ? (
          <div className="flex flex-col gap-2">
            <p className="text-small text-ink">
              {t("plan.meal.taken", {
                name: taken.name,
                cell: cellLabel(taken, t, i18n, slotName),
              })}
            </p>
            <Button
              type="button"
              className={button}
              onClick={() => {
                onChoose(taken, "swap")
                close(false)
              }}
            >
              {t("plan.meal.swap")}
            </Button>
            <Button
              type="button"
              variant="outline"
              className={button}
              onClick={() => {
                onChoose(taken, "drop")
                close(false)
              }}
            >
              {t("plan.meal.replace", { name: taken.name })}
            </Button>
            <Button
              type="button"
              variant="ghost"
              className={button}
              onClick={() => setTaken(null)}
            >
              {t("common.cancel")}
            </Button>
          </div>
        ) : (
          <CellList
            cells={cells}
            slotName={slotName}
            onChoose={(c) => {
              if (c.plateId !== null) {
                setTaken(c)
                return
              }
              onChoose(c)
              close(false)
            }}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}
