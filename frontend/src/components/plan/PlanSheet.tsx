import { X } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import type { Plan } from "@/lib/api/plan"
import type { Plate } from "@/lib/api/plates"
import type { Board, BoardCell } from "@/lib/plan/board"
import { formatDate } from "@/lib/plan/format"
import { slotLabel } from "@/lib/slot-label"
import { useSettingField } from "@/components/settings/setting-field"
import { cn } from "@/lib/utils"

import { CellSheet } from "./CellSheet"
import { FoodPicker, type PickerTarget } from "./FoodPicker"
import { PlateSheet } from "./PlateSheet"
import { useIsPhone } from "./use-is-phone"

/** Which cell the sheet shows, and whether its food picker is open. */
export interface SheetState {
  key: string
  picker?: PickerTarget
}

interface PlanSheetProps {
  plan: Plan
  board: Board
  state: SheetState | null
  /** A generation runs for this window: regenerating waits for it. */
  running: boolean
  onChange: (state: SheetState | null) => void
  /** After the sheet closed: return focus to the cell it was opened for. */
  onClosed: () => void
}

function findCell(board: Board, key: string): BoardCell | undefined {
  for (const day of board.days) {
    const cell = day.cells.find((c) => c.key === key)
    if (cell) return cell
  }
  return undefined
}

/**
 * The one sheet of the board (UX §5.5, §6 "never stack two overlays"): a
 * side sheet from 768 px and a bottom sheet on phones. It shows the plate
 * sheet, the empty-cell sheet or the food picker for one cell, always from
 * the latest plan, so it follows every refetch.
 */
export function PlanSheet({
  plan,
  board,
  state,
  running,
  onChange,
  onClosed,
}: PlanSheetProps) {
  const { t, i18n } = useTranslation()
  const phone = useIsPhone()
  const freezing = useSettingField("plan.freezing_enabled", "false")
  const cell = state ? findCell(board, state.key) : undefined
  const raw = cell
    ? plan.cells.find((c) => c.date === cell.date && c.slot_id === cell.slotId)
    : undefined
  const slotNames = new Map(
    plan.slots.map((s) => [s.id, slotLabel(t, s.name_key)])
  )
  const slotName = (id: number) => slotNames.get(id) ?? ""
  const close = () => onChange(null)

  const where = cell
    ? t("plan.cell.name", {
        weekday: formatDate(cell.date, i18n, { weekday: "long" }),
        slot: slotName(cell.slotId),
      })
    : ""
  const picker = state?.picker
  const title = picker
    ? picker.kind === "swap"
      ? t("plan.picker.title_swap", { name: picker.name })
      : t("plan.picker.title_add")
    : (cell?.mainName ?? where)

  return (
    <Sheet open={!!cell} onOpenChange={(open) => !open && close()}>
      <SheetContent
        side={phone ? "bottom" : "right"}
        className={cn(
          "flex flex-col gap-0 bg-panel p-0 text-ink shadow-sheet",
          phone
            ? "max-h-[90dvh] rounded-t-sheet"
            : "w-full sm:max-w-md md:max-w-md"
        )}
        data-testid="plan-sheet"
        showCloseButton={false}
        onCloseAutoFocus={(e) => {
          // No trigger to return to: focus the cell the sheet was for.
          e.preventDefault()
          onClosed()
        }}
      >
        <SheetClose asChild>
          <Button
            type="button"
            variant="ghost"
            className="absolute top-3 right-3 size-11 md:pointer-fine:size-8"
            aria-label={t("common.close")}
            data-testid="plan-sheet-close"
          >
            <X strokeWidth={1.5} aria-hidden />
          </Button>
        </SheetClose>
        <SheetHeader className="px-4 pt-5 pr-14 pb-3">
          <SheetTitle className="font-heading text-h2 font-normal text-ink">
            {title}
          </SheetTitle>
          <SheetDescription className="text-small text-ink-muted first-letter:uppercase">
            {where}
          </SheetDescription>
        </SheetHeader>
        {cell &&
          raw &&
          state &&
          (picker ? (
            <FoodPicker
              key={JSON.stringify(picker)}
              target={picker}
              onDone={() => onChange({ key: state.key })}
              onCancel={() => onChange({ key: state.key })}
            />
          ) : cell.plate ? (
            <PlateSheet
              key={cell.plate.id}
              cell={cell as BoardCell & { plate: Plate }}
              plan={plan}
              readOnly={board.readOnly}
              running={running}
              freezing={freezing.value === "true"}
              slotName={slotName}
              onPick={(p) => onChange({ key: state.key, picker: p })}
              onOpenCell={(key) => onChange({ key })}
              onClose={close}
            />
          ) : (
            <CellSheet
              key={cell.key}
              cell={raw}
              plan={plan}
              onPick={(p) => onChange({ key: state.key, picker: p })}
              onClose={close}
            />
          ))}
      </SheetContent>
    </Sheet>
  )
}
