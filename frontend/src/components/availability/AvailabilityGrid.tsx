import { MoreHorizontal } from "lucide-react"
import { useRef, useState, type KeyboardEvent } from "react"
import { useTranslation } from "react-i18next"

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { AvailabilityMode } from "@/lib/api/availability"
import { cn } from "@/lib/utils"

import { RoutinePicker, type RoutinePreset } from "./RoutinePicker"

export interface AvailabilityGridColumn {
  key: string
  /** Shown in the header; may be short ("Mon"). */
  label: string
  /** Full name for headers and cell names read by assistive tech ("Monday"). */
  fullLabel?: string
  sublabel?: string
}

export interface AvailabilityGridSlot {
  id: number
  label: string
}

export interface AvailabilityCellValue {
  mode: AvailabilityMode
  presetId?: number
}

/** Keyed by `"<colKey>:<slotId>"`. A missing key means `plan`. */
export type AvailabilityGridValue = Record<string, AvailabilityCellValue>

export interface AvailabilityGridProps {
  /** Accessible name of the grid, e.g. "Default week". */
  label: string
  columns: AvailabilityGridColumn[]
  slots: AvailabilityGridSlot[]
  value: AvailabilityGridValue
  presets: RoutinePreset[]
  onChange: (
    colKey: string,
    slotId: number,
    next: AvailabilityCellValue
  ) => void
  /** When given, each column header gets a menu with "No plate all day" /
   *  "Plan all day"; it receives one patch covering every slot of the column. */
  onBulkChange?: (patch: AvailabilityGridValue) => void
  disabled?: boolean
  /** Background the grid sits on, so the sticky slot column matches it. */
  surface?: "ground" | "panel"
}

const PLAN: AvailabilityCellValue = { mode: "plan" }

const cellKey = (colKey: string, slotId: number) => `${colKey}:${slotId}`

/**
 * Presentational weekday (or date) × slot grid of availability modes. A tap
 * cycles Plan → No plate → Routine → Plan. Routine first asks for a preset in
 * `RoutinePicker`; closing the picker without a pick (Cancel, Escape, outside
 * click) skips Routine and lands on Plan, so No plate is never a dead end.
 * With no presets the cycle is Plan ↔ No plate. It owns no server state: the
 * parent holds `value`.
 */
export function AvailabilityGrid({
  label,
  columns,
  slots,
  value,
  presets,
  onChange,
  onBulkChange,
  disabled = false,
  surface = "ground",
}: AvailabilityGridProps) {
  const { t } = useTranslation()
  const [active, setActive] = useState({ row: 0, col: 0 })
  const [pickerFor, setPickerFor] = useState<string | null>(null)
  const cellRefs = useRef(new Map<string, HTMLButtonElement>())

  const presetNames = new Map(presets.map((p) => [p.id, p.name]))
  const activeRow = Math.min(active.row, slots.length - 1)
  const activeCol = Math.min(active.col, columns.length - 1)
  const stickyBg = surface === "panel" ? "bg-panel" : "bg-ground"

  function stateText(cell: AvailabilityCellValue) {
    if (cell.mode === "none") return t("availability.mode.none")
    if (cell.mode === "plan") return t("availability.mode.plan")
    const name = cell.presetId != null ? presetNames.get(cell.presetId) : null
    return name
      ? t("availability.routine_named", { name })
      : t("availability.mode.routine")
  }

  function focusCell(row: number, col: number) {
    setActive({ row, col })
    cellRefs.current.get(cellKey(columns[col].key, slots[row].id))?.focus()
  }

  function handleKeyDown(e: KeyboardEvent, row: number, col: number) {
    const lastRow = slots.length - 1
    const lastCol = columns.length - 1
    const moves: Record<string, [number, number]> = {
      ArrowRight: [row, Math.min(col + 1, lastCol)],
      ArrowLeft: [row, Math.max(col - 1, 0)],
      ArrowDown: [Math.min(row + 1, lastRow), col],
      ArrowUp: [Math.max(row - 1, 0), col],
      Home: [row, 0],
      End: [row, lastCol],
    }
    const target = moves[e.key]
    if (!target) return
    e.preventDefault()
    focusCell(target[0], target[1])
  }

  const canRoutine = presets.length > 0

  function handleCellClick(colKey: string, slotId: number) {
    const current = value[cellKey(colKey, slotId)] ?? PLAN
    if (current.mode === "plan") onChange(colKey, slotId, { mode: "none" })
    else if (current.mode === "none" && canRoutine)
      setPickerFor(cellKey(colKey, slotId))
    else onChange(colKey, slotId, PLAN)
  }

  function bulk(colKey: string, mode: "plan" | "none") {
    onBulkChange?.(
      Object.fromEntries(slots.map((s) => [cellKey(colKey, s.id), { mode }]))
    )
  }

  return (
    // `relative` makes this scroll box the containing block of the cells'
    // sr-only labels, so they scroll with it instead of widening the page.
    <div className="relative overflow-x-auto overscroll-x-contain p-1">
      {/* 32rem = slot column (5rem) + seven 3.5rem day columns + spacing:
          the narrowest width where "Mon ⋯", "Routine" and each wrapped word
          of "Kein Essen" still fit. Below it the box scrolls; above it the
          day columns share the extra width. */}
      <table
        role="grid"
        aria-label={label}
        className="w-full min-w-[32rem] table-fixed border-separate border-spacing-1"
      >
        <colgroup>
          <col className="w-20" />
          {columns.map((c) => (
            <col key={c.key} />
          ))}
        </colgroup>
        <thead>
          <tr role="row">
            <th
              role="columnheader"
              scope="col"
              className={cn("sticky left-0 z-10", stickyBg)}
            >
              <span className="sr-only">{t("availability.slot_header")}</span>
            </th>
            {columns.map((col) => (
              <th
                key={col.key}
                role="columnheader"
                scope="col"
                className="pb-1 pl-1 text-left align-bottom font-normal"
              >
                <div className="flex items-end justify-between gap-0.5">
                  <span className="flex min-w-0 flex-col">
                    <span
                      aria-hidden={col.fullLabel ? true : undefined}
                      className="truncate text-small font-semibold text-ink"
                    >
                      {col.label}
                    </span>
                    {col.fullLabel ? (
                      <span className="sr-only">{col.fullLabel}</span>
                    ) : null}
                    {col.sublabel ? (
                      <span className="truncate text-caption text-ink-muted tabular-nums">
                        {col.sublabel}
                      </span>
                    ) : null}
                  </span>
                  {onBulkChange ? (
                    <DropdownMenu>
                      <DropdownMenuTrigger
                        disabled={disabled}
                        aria-label={t("availability.column_actions", {
                          column: col.fullLabel ?? col.label,
                        })}
                        className="relative flex size-6 shrink-0 items-center justify-center rounded-cell text-ink-muted after:absolute after:inset-0 hover:bg-panel-sunken hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent disabled:opacity-50 pointer-coarse:after:-top-5 pointer-coarse:after:-left-5"
                      >
                        <MoreHorizontal
                          aria-hidden
                          strokeWidth={1.5}
                          className="size-4"
                        />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuGroup>
                          <DropdownMenuItem
                            onSelect={() => bulk(col.key, "none")}
                          >
                            {t("availability.bulk_none")}
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onSelect={() => bulk(col.key, "plan")}
                          >
                            {t("availability.bulk_plan")}
                          </DropdownMenuItem>
                        </DropdownMenuGroup>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  ) : null}
                </div>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {slots.map((slot, row) => (
            <tr key={slot.id} role="row">
              <th
                role="rowheader"
                scope="row"
                className={cn(
                  "sticky left-0 z-10 pr-2 text-left text-small font-normal wrap-break-word hyphens-auto text-ink-muted",
                  stickyBg
                )}
              >
                {slot.label}
              </th>
              {columns.map((col, colIndex) => {
                const key = cellKey(col.key, slot.id)
                const cell = value[key] ?? PLAN
                const state = stateText(cell)
                const presetName =
                  cell.presetId != null ? presetNames.get(cell.presetId) : null
                return (
                  <td key={col.key} role="gridcell" className="p-0">
                    <RoutinePicker
                      open={pickerFor === key}
                      onOpenChange={(open) => {
                        if (open) return
                        // Closed without a pick: skip Routine, back to Plan.
                        setPickerFor(null)
                        onChange(col.key, slot.id, PLAN)
                      }}
                      presets={presets}
                      selectedId={cell.presetId}
                      onPick={(presetId) => {
                        setPickerFor(null)
                        onChange(col.key, slot.id, {
                          mode: "routine",
                          presetId,
                        })
                      }}
                      onCloseAutoFocus={(e) => {
                        e.preventDefault()
                        cellRefs.current.get(key)?.focus()
                      }}
                    >
                      <button
                        type="button"
                        ref={(el) => {
                          if (el) cellRefs.current.set(key, el)
                          else cellRefs.current.delete(key)
                        }}
                        tabIndex={
                          row === activeRow && colIndex === activeCol ? 0 : -1
                        }
                        disabled={disabled}
                        aria-label={t("availability.cell_name", {
                          column: col.fullLabel ?? col.label,
                          slot: slot.label,
                          slotLower: slot.label.toLowerCase(),
                          state,
                        })}
                        aria-haspopup={
                          cell.mode === "none" && canRoutine
                            ? "dialog"
                            : undefined
                        }
                        aria-expanded={
                          cell.mode === "none" && canRoutine
                            ? pickerFor === key
                            : undefined
                        }
                        onFocus={() => setActive({ row, col: colIndex })}
                        onKeyDown={(e) => handleKeyDown(e, row, colIndex)}
                        onClick={() => handleCellClick(col.key, slot.id)}
                        className={cn(
                          "flex min-h-11 w-full min-w-0 flex-col items-start justify-center rounded-cell border px-1.5 py-1 text-left text-small leading-tight transition-colors duration-150 ease-out",
                          "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
                          "disabled:cursor-not-allowed disabled:opacity-50",
                          cell.mode === "none"
                            ? "border-transparent bg-transparent text-ink-muted italic hover:border-line"
                            : "border-line bg-panel hover:border-ink-muted",
                          cell.mode === "plan" && "text-ink-muted"
                        )}
                      >
                        {cell.mode === "routine" ? (
                          <>
                            <span className="text-caption text-ink-muted">
                              {t("availability.mode.routine")}
                            </span>
                            {presetName ? (
                              <span className="line-clamp-2 w-full wrap-anywhere text-ink">
                                {presetName}
                              </span>
                            ) : null}
                          </>
                        ) : (
                          <span className="w-full wrap-break-word">
                            {state}
                          </span>
                        )}
                      </button>
                    </RoutinePicker>
                  </td>
                )
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
