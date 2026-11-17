import {
  DndContext,
  MouseSensor,
  pointerWithin,
  TouchSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core"
import { useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { movePlate, type Occupant } from "@/lib/api/meal"
import type { Plan } from "@/lib/api/plan"
import { todayISO } from "@/lib/dates"
import type { BoardCell } from "@/lib/plan/board"
import { copySnapshot } from "@/lib/plan/board"
import { cellLabel, targetCells } from "@/lib/plan/meal-actions"
import { useMovePlate } from "@/lib/queries/meal"
import { useRestorePlan } from "@/lib/queries/plan"
import { toast } from "@/lib/toast"

import { BoardGrid, type BoardLayoutProps } from "./BoardGrid"
import { DayList } from "./DayList"
import { useAlsoChangedConfirm } from "./MealActions"
import { useIsPhone } from "./use-is-phone"

function cellOf(data: unknown): BoardCell | undefined {
  return (data as { cell?: BoardCell } | undefined)?.cell
}

interface Drop {
  from: BoardCell
  to: BoardCell
  x: number
  y: number
}

/**
 * The board: the day list on phones, the grid from 768 px, both from the
 * same view model (UX §9 "one board, three layouts"). Owns drag and drop:
 * a drag moves the plate (with Undo, and the also-changed preview when it
 * changes other meals); a drop onto a taken cell opens a small menu at the
 * target (Swap · Move X to… · Replace X · Cancel). Holding Ctrl/⌘ when the
 * drag starts copies the plate as a fresh meal through restore. Drops onto
 * No plate cells are refused before any request.
 */
export function Board({
  plan,
  slotName,
  ...props
}: BoardLayoutProps & { plan: Plan; slotName: (id: number) => string }) {
  const { t, i18n } = useTranslation()
  const phone = useIsPhone()
  const { confirm, describe, dialog } = useAlsoChangedConfirm(plan, slotName)
  const move = useMovePlate(describe)
  const restore = useRestorePlan()
  const copy = useRef(false)
  // The cell the menu opened at: focus returns there when the menu closes.
  const menuTarget = useRef<string | null>(null)
  const [drop, setDrop] = useState<Drop | null>(null)

  const sensors = useSensors(
    // A short move separates a drag from a click that opens the cell.
    useSensor(MouseSensor, { activationConstraint: { distance: 6 } }),
    // Touch (tablet) needs a press-and-hold so swiping still scrolls.
    useSensor(TouchSensor, {
      activationConstraint: { delay: 250, tolerance: 5 },
    })
  )

  /** Moves `from`'s plate to `to`, previewing what else changes first. */
  function moveTo(
    from: BoardCell,
    to: BoardCell | { date: string; slotId: number },
    occupant?: Occupant
  ) {
    const id = from.plate!.id
    const input = { date: to.date, slot_id: to.slotId, occupant }
    confirm(
      () => movePlate(id, input, true).then((r) => r.also_changed),
      () => move.mutate({ id, input })
    )
  }

  function onDragStart(e: DragStartEvent) {
    const a = e.activatorEvent as MouseEvent | TouchEvent | null
    copy.current = !!a && (a.ctrlKey || a.metaKey)
  }

  function onDragEnd(e: DragEndEvent) {
    const from = cellOf(e.active.data.current)
    const to = cellOf(e.over?.data.current)
    if (!from?.plate || !to || to.key === from.key) return
    if (to.mode === "none") {
      toast.error(t("plan.dnd.reject_no_plate"))
      return
    }
    if (to.plate) {
      if (copy.current) {
        toast.error(t("plan.dnd.reject_occupied"))
        return
      }
      const r = e.over!.rect
      menuTarget.current = to.key
      setDrop({ from, to, x: r.left + r.width / 2, y: r.top + r.height / 2 })
      return
    }
    if (copy.current) {
      restore.mutate(copySnapshot(from.plate, to.date, to.slotId))
    } else {
      moveTo(from, to)
    }
  }

  const occupantName = drop?.to.mainName ?? ""
  const free = drop
    ? targetCells(plan, todayISO(), { except: drop.from.key })
    : []

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={pointerWithin}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
    >
      {phone ? <DayList {...props} /> : <BoardGrid {...props} />}
      <DropdownMenu
        open={drop !== null}
        onOpenChange={(o) => !o && setDrop(null)}
      >
        <DropdownMenuTrigger asChild>
          <span
            aria-hidden
            className="pointer-events-none fixed size-px"
            style={{ left: drop?.x ?? 0, top: drop?.y ?? 0 }}
          />
        </DropdownMenuTrigger>
        <DropdownMenuContent
          data-testid="occupied-menu"
          className="max-w-[calc(100vw-2rem)]"
          onCloseAutoFocus={(e) => {
            // The menu's trigger is an invisible marker: focus the cell.
            const cell = document.querySelector<HTMLElement>(
              `[data-cell-key="${menuTarget.current}"] [data-cell-focus]`
            )
            if (!cell) return
            e.preventDefault()
            cell.focus()
          }}
        >
          <DropdownMenuGroup>
            <DropdownMenuItem
              onSelect={() => drop && moveTo(drop.from, drop.to, "swap")}
            >
              {t("plan.meal.swap")}
            </DropdownMenuItem>
            <DropdownMenuSub>
              <DropdownMenuSubTrigger disabled={free.length === 0}>
                {t("plan.meal.move_occupant", { name: occupantName })}
              </DropdownMenuSubTrigger>
              <DropdownMenuSubContent className="max-h-[50vh] overflow-y-auto">
                {free.map((c) => (
                  <DropdownMenuItem
                    key={c.key}
                    onSelect={() =>
                      drop &&
                      moveTo(drop.from, drop.to, {
                        date: c.date,
                        slot_id: c.slot_id,
                      })
                    }
                  >
                    {cellLabel(c, t, i18n, slotName)}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuSubContent>
            </DropdownMenuSub>
            <DropdownMenuItem
              onSelect={() => drop && moveTo(drop.from, drop.to, "drop")}
            >
              {t("plan.meal.replace", { name: occupantName })}
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={() => setDrop(null)}>
              {t("common.cancel")}
            </DropdownMenuItem>
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
      {dialog}
    </DndContext>
  )
}
