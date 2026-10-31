/**
 * Grid keyboard navigation for the plan board. Cells are tagged with
 * data-cell-pos="row-col" (row = slot index, col = day index 0-6) and hold
 * one element marked `data-cell-focus`. The grid container delegates
 * onKeyDown to {@link handleGridArrowKey}, which moves focus to that element
 * of the target cell.
 *
 * The handler is intentionally pure-DOM: it doesn't need React state because
 * focus is the source of truth. Keeping it out of React state also avoids a
 * setState-in-keyDown anti-pattern.
 */

export interface GridKeyNavOptions {
  rows: number
  cols: number
}

function focusableInCell(cell: Element): HTMLElement | null {
  return cell.querySelector("[data-cell-focus]") as HTMLElement | null
}

function findCellByPos(
  root: Element,
  row: number,
  col: number
): HTMLElement | null {
  return root.querySelector(
    `[data-cell-pos="${row}-${col}"]`
  ) as HTMLElement | null
}

/** Reads "row-col" off the closest ancestor of `target` that carries it. */
export function readCellPos(
  target: EventTarget | null
): { row: number; col: number } | null {
  if (!(target instanceof Element)) return null
  const cell = target.closest("[data-cell-pos]")
  if (!cell) return null
  const raw = cell.getAttribute("data-cell-pos")
  if (!raw) return null
  const [rStr, cStr] = raw.split("-")
  const row = Number(rStr)
  const col = Number(cStr)
  if (!Number.isFinite(row) || !Number.isFinite(col)) return null
  return { row, col }
}

/**
 * Returns true if the event was handled (caller should not let it bubble).
 * Handles ArrowLeft/Right/Up/Down and Home/End. Left and right read the
 * grid in order: past a row end they wrap to the next (or previous) row,
 * and from the last cell to the first. Up and down wrap within the column.
 */
export function handleGridArrowKey(
  e: React.KeyboardEvent,
  opts: GridKeyNavOptions
): boolean {
  if (e.metaKey || e.ctrlKey || e.altKey) return false
  const pos = readCellPos(e.target)
  if (!pos) return false
  const { rows, cols } = opts
  let { row, col } = pos
  switch (e.key) {
    case "ArrowLeft":
      if (col === 0) row = (row - 1 + rows) % rows
      col = (col - 1 + cols) % cols
      break
    case "ArrowRight":
      if (col === cols - 1) row = (row + 1) % rows
      col = (col + 1) % cols
      break
    case "ArrowUp":
      row = (row - 1 + rows) % rows
      break
    case "ArrowDown":
      row = (row + 1) % rows
      break
    case "Home":
      col = 0
      break
    case "End":
      col = cols - 1
      break
    default:
      return false
  }
  const root = e.currentTarget as Element
  const target = findCellByPos(root, row, col)
  const focusable = target ? focusableInCell(target) : null
  if (!focusable) return false
  e.preventDefault()
  focusable.focus()
  return true
}

/** True when typing in `target` means the literal character. */
function isEditable(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return false
  const tag = target.tagName
  if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return true
  if (target instanceof HTMLElement) {
    if (target.isContentEditable) return true
    // jsdom doesn't always wire isContentEditable from the attribute, so
    // fall back to the attribute itself for tests and old browsers.
    const attr = target.getAttribute("contenteditable")
    if (attr === "" || attr === "true") return true
  }
  return false
}

/**
 * `?` (Shift+/) opens the cheatsheet from anywhere on the board — except
 * when an editable surface owns focus, where the keystroke means the literal
 * character. Returns true if the shortcut fired so the caller can preventDefault.
 */
export function isCheatsheetShortcut(e: KeyboardEvent): boolean {
  if (e.key !== "?") return false
  if (e.metaKey || e.ctrlKey || e.altKey) return false
  return !isEditable(e.target)
}

/** `⌘/Ctrl+Enter` accepts the draft (UX §6), outside editable fields and
 *  dialogs (sheets included): there the keys belong to the overlay. */
export function isAcceptShortcut(e: KeyboardEvent): boolean {
  if (e.key !== "Enter" || e.altKey || e.shiftKey) return false
  if (!e.metaKey && !e.ctrlKey) return false
  if (e.target instanceof Element && e.target.closest('[role="dialog"]')) {
    return false
  }
  return !isEditable(e.target)
}

/** What a letter key does on a focused board cell (UX §6). */
export type CellShortcut = "no_plate" | "replace" | "side"

const CELL_KEYS: Record<string, CellShortcut> = {
  n: "no_plate",
  r: "replace",
  s: "side",
}

/** N marks No plate, R replaces, S changes the side; null for any other
 *  key, and for these with a modifier (⌘R reloads the page). */
export function cellShortcut(
  e: Pick<KeyboardEvent, "key" | "metaKey" | "ctrlKey" | "altKey">
): CellShortcut | null {
  if (e.metaKey || e.ctrlKey || e.altKey) return null
  return CELL_KEYS[e.key.toLowerCase()] ?? null
}
