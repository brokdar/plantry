import type { AvailabilityMode } from "@/lib/api/availability"
import type { Plan, PlanCell, PlanDay } from "@/lib/api/plan"
import type { PlanSnapshot, Plate, PlateComponent } from "@/lib/api/plates"
import type { SessionWarningCode } from "@/lib/api/sessions"
import {
  colourBatches,
  type BatchColour,
  type ColouredBatch,
} from "@/lib/plan/batch-colours"
import {
  batchBadgeText,
  formatAmount,
  formatDate,
  type BatchBadgeContent,
  type Formatter,
} from "@/lib/plan/format"
import { slotLabel } from "@/lib/slot-label"

/** The badge a cell shows for the batch its main food comes from. */
export type CellBadge = BatchBadgeContent & {
  sessionId: number
  colour: BatchColour
}

/** The muted line under a cell's name (UX §5.5). */
export type CellSubline = "cook" | "leftover" | "routine"

export interface BoardCell {
  /** `date|slotId`, unique within the window. */
  key: string
  date: string
  slotId: number
  mode: AvailabilityMode
  plate: Plate | null
  badge: CellBadge | null
  /** The main food's name; null without a plate. */
  mainName: string | null
  /** "with rice and salad" for the other components; null when none. */
  sideText: string | null
  subline: CellSubline | null
  /** Routine cells: the routine's name. */
  presetName: string | null
  /** No-plate cells: the reason, if given. */
  note: string | null
  warnings: SessionWarningCode[]
  /** Sessions the plate eats from (for the linked highlight). */
  sessionIds: number[]
  /** Sessions cooked in this cell (the cook strip scrolls to them). */
  cookOf: number[]
  /** Full name, e.g. "Monday dinner, Chili, leftover 3 of 3". */
  ariaLabel: string
}

export interface BoardDay {
  date: string
  /** "Sat". */
  weekdayShort: string
  /** "31". */
  dayNumber: string
  /** "Saturday". */
  weekday: string
  isToday: boolean
  nutrition: PlanDay | null
  /** The day's cells in slot order. */
  cells: BoardCell[]
}

export interface BoardRow {
  slotId: number
  name: string
  /** Set when every cell of the row is a routine without a plate:
   *  "Breakfast: Oats every day". The row then renders as one line. */
  collapsed: string | null
  /** The row's cells in day order. */
  cells: BoardCell[]
}

export interface Board {
  days: BoardDay[]
  rows: BoardRow[]
  /** Batches cooked in this window, in cook order, with their colours. */
  cookStrip: ColouredBatch[]
  /** Cell keys that need attention, in board order (day, then slot). */
  warningCells: string[]
  /** Past windows: no edits, feedback only. */
  readOnly: boolean
  /** No plate anywhere in the window. */
  empty: boolean
}

const cellKey = (date: string, slotId: number) => `${date}|${slotId}`

/** The component that names the plate: the main, else the first composed,
 *  else the first one. */
function mainComponent(components: PlateComponent[]): PlateComponent | null {
  return (
    components.find((c) => c.role === "main") ??
    components.find((c) => c.kind === "composed") ??
    components[0] ??
    null
  )
}

function sortedComponents(plate: Plate): PlateComponent[] {
  return [...plate.components].sort((a, b) => a.sort_order - b.sort_order)
}

/**
 * The board's view model (UX §5.5, D23): the grid and the phone day list
 * render only from this. Batch geometry, consumption and warnings come from
 * the API; this only joins and labels them.
 */
export function buildBoard(plan: Plan, f: Formatter): Board {
  const coloured = colourBatches(plan.sessions, plan.slots)
  const byId = new Map(coloured.map((b) => [b.session.id, b]))
  const slots = [...plan.slots].sort(
    (a, b) => a.sort_order - b.sort_order || a.id - b.id
  )
  const slotName = new Map(slots.map((s) => [s.id, slotLabel(f.t, s.name_key)]))
  const cells = new Map(plan.cells.map((c) => [cellKey(c.date, c.slot_id), c]))
  const dates = windowDates(plan.window.start, plan.window.end)
  const nutrition = new Map(plan.days.map((d) => [d.date, d]))

  // Warnings per plate; a session-wide one (plate_id null) sits on the cook
  // cell.
  const warningsByPlate = new Map<number, SessionWarningCode[]>()
  const warningsByCookCell = new Map<string, SessionWarningCode[]>()
  for (const s of plan.sessions) {
    for (const w of s.warnings) {
      if (w.plate_id !== null) {
        push(warningsByPlate, w.plate_id, w.code)
      } else {
        push(warningsByCookCell, cellKey(s.cook_date, s.cook_slot_id), w.code)
      }
    }
  }

  const listFormat = new Intl.ListFormat(f.language, { type: "conjunction" })

  function buildCell(date: string, slotId: number): BoardCell {
    const key = cellKey(date, slotId)
    const raw: PlanCell | undefined = cells.get(key)
    const mode = raw?.mode ?? "plan"
    const plate = raw?.plate ?? null
    const components = plate ? sortedComponents(plate) : []
    const main = mainComponent(components)
    const sides = components.filter((c) => c !== main).map((c) => c.food_name)

    const sessionIds = [
      ...new Set(
        components.flatMap((c) =>
          c.cook_session_id != null ? [c.cook_session_id] : []
        )
      ),
    ]
    const cookOf = plate
      ? sessionIds.filter((id) =>
          byId
            .get(id)
            ?.session.links.some(
              (l) => l.plate_id === plate.id && l.kind === "fresh"
            )
        )
      : []

    let badge: CellBadge | null = null
    if (plate && main?.cook_session_id != null) {
      const batch = byId.get(main.cook_session_id)
      const link = batch?.session.links.find((l) => l.component_id === main.id)
      if (batch && link) {
        const base = { sessionId: batch.session.id, colour: batch.colour }
        badge =
          link.kind === "fresh"
            ? { ...base, variant: "cook", portions: batch.session.portions }
            : {
                ...base,
                variant: link.kind,
                position: link.position,
                meals: batch.session.meals,
              }
      }
    }

    const subline: CellSubline | null = badge
      ? badge.variant === "cook"
        ? "cook"
        : "leftover"
      : mode === "routine"
        ? "routine"
        : null

    const warnings = [
      ...(plate ? (warningsByPlate.get(plate.id) ?? []) : []),
      ...(plate ? (warningsByCookCell.get(key) ?? []) : []),
    ]

    const cell: BoardCell = {
      key,
      date,
      slotId,
      mode,
      plate,
      badge,
      mainName: main?.food_name ?? null,
      sideText:
        sides.length > 0
          ? f.t("plan.cell.with", { names: listFormat.format(sides) })
          : null,
      subline,
      presetName: raw?.preset_name ?? null,
      note: raw?.note ?? null,
      warnings,
      sessionIds,
      cookOf,
      ariaLabel: "",
    }
    cell.ariaLabel = cellAriaLabel(cell, slotName.get(slotId) ?? "", f)
    return cell
  }

  const grid = dates.map((date) => slots.map((s) => buildCell(date, s.id)))

  const days: BoardDay[] = dates.map((date, i) => ({
    date,
    weekdayShort: formatDate(date, f, { weekday: "short" }),
    dayNumber: formatDate(date, f, { day: "numeric" }),
    weekday: formatDate(date, f, { weekday: "long" }),
    isToday: date === plan.window.today,
    nutrition: nutrition.get(date) ?? null,
    cells: grid[i],
  }))

  const rows: BoardRow[] = slots.map((slot, si) => {
    const rowCells = grid.map((dayCells) => dayCells[si])
    const name = slotName.get(slot.id) ?? ""
    return {
      slotId: slot.id,
      name,
      collapsed: collapsedText(rowCells, name, f),
      cells: rowCells,
    }
  })

  const warningCells = days.flatMap((d) =>
    d.cells.filter((c) => c.warnings.length > 0).map((c) => c.key)
  )

  return {
    days,
    rows,
    cookStrip: coloured.filter(
      ({ session }) =>
        session.cook_date >= plan.window.start &&
        session.cook_date <= plan.window.end
    ),
    warningCells,
    readOnly: plan.window.end < plan.window.today,
    empty: !plan.cells.some((c) => c.plate),
  }
}

/** "Breakfast: Oats every day" when every cell is a routine without a
 *  plate; null otherwise. */
function collapsedText(
  cells: BoardCell[],
  slotName: string,
  f: Formatter
): string | null {
  if (cells.length === 0) return null
  if (!cells.every((c) => c.mode === "routine" && !c.plate)) return null
  const names = new Set(cells.map((c) => c.presetName))
  const routine =
    names.size === 1 && cells[0].presetName
      ? cells[0].presetName
      : f.t("plan.cell.routine")
  return f.t("plan.row.collapsed", { slot: slotName, routine })
}

function cellAriaLabel(cell: BoardCell, slot: string, f: Formatter): string {
  const parts = [
    f.t("plan.cell.name", {
      weekday: formatDate(cell.date, f, { weekday: "long" }),
      slot,
    }),
  ]
  if (cell.plate) {
    if (cell.mainName) parts.push(cell.mainName)
    if (cell.sideText) parts.push(cell.sideText)
    if (cell.badge) parts.push(badgeSpeech(cell.badge, f))
    else if (!cell.mainName) parts.push(f.t("plan.cell.empty_plate"))
  } else if (cell.mode === "none") {
    parts.push(f.t("plan.cell.no_plate"))
    if (cell.note) parts.push(cell.note)
  } else if (cell.mode === "routine") {
    parts.push(f.t("plan.cell.routine"))
    if (cell.presetName) parts.push(cell.presetName)
  } else {
    parts.push(f.t("plan.cell.nothing_planned"))
  }
  if (cell.warnings.length > 0) parts.push(f.t("plan.cell.needs_attention"))
  return parts.join(", ")
}

/** The badge read out in words: "cook here, 6 portions", "leftover 3 of 3". */
function badgeSpeech(badge: CellBadge, f: Formatter): string {
  switch (badge.variant) {
    case "cook":
      return f.t("plan.cell.speech_cook", {
        count: badge.portions,
        portions: formatAmount(badge.portions, f),
      })
    case "leftover":
      return f.t("plan.cell.speech_leftover", {
        position: badge.position,
        meals: badge.meals,
      })
    case "frozen":
      return f.t("plan.cell.speech_frozen", {
        position: badge.position,
        meals: badge.meals,
      })
    default:
      return batchBadgeText(badge, f)
  }
}

/**
 * The one-plate snapshot a Ctrl/⌘-drag copy restores at (date, slotId).
 * Components carry no `session`: a copy is a new fresh meal (D8), so the
 * server gives each composed component its own session. Feedback stays
 * with the original.
 */
export function copySnapshot(
  plate: Plate,
  date: string,
  slotId: number
): PlanSnapshot {
  return {
    plates: [
      {
        date,
        slot_id: slotId,
        eaters: plate.eaters,
        note: plate.note,
        components: sortedComponents(plate).map((c) =>
          c.kind === "leaf"
            ? {
                food_id: c.food_id,
                amount: c.amount,
                unit: c.unit,
                grams: c.grams,
                sort_order: c.sort_order,
              }
            : {
                food_id: c.food_id,
                portions: c.portions,
                sort_order: c.sort_order,
              }
        ),
      },
    ],
    sessions: [],
  }
}

function push<K, V>(map: Map<K, V[]>, key: K, value: V) {
  const list = map.get(key)
  if (list) list.push(value)
  else map.set(key, [value])
}

/** The dates from start to end inclusive (YYYY-MM-DD, calendar days). */
function windowDates(start: string, end: string): string[] {
  const out: string[] = []
  const [y, m, d] = start.split("-").map(Number)
  for (let i = 0; i < 31; i++) {
    const day = new Date(Date.UTC(y, m - 1, d + i)).toISOString().slice(0, 10)
    if (day > end) break
    out.push(day)
  }
  return out
}
