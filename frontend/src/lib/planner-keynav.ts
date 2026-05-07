/**
 * Grid keyboard navigation for the planner. Cells are tagged with
 * data-cell-pos="row-col" (row = slot index, col = day index 0-6). The grid
 * container delegates onKeyDown to {@link handleGridArrowKey}, which moves
 * focus to the right cell's primary interactive element.
 *
 * The handler is intentionally pure-DOM: it doesn't need React state because
 * focus is the source of truth. Keeping it out of React state also avoids a
 * setState-in-keyDown anti-pattern.
 */

export interface GridKeyNavOptions {
  rows: number
  cols: number
}

const FOCUS_SELECTOR =
  'button[data-testid="slot-open-sheet"], button[data-testid="slot-empty-add"], [data-slot-state="skipped"]'

function focusableInCell(cell: Element): HTMLElement | null {
  return cell.querySelector(FOCUS_SELECTOR) as HTMLElement | null
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
 * Handles ArrowLeft/Right/Up/Down and Home/End. Wraps within the grid: e.g.
 * pressing ArrowRight on the last column moves to the first column of the
 * same row, matching common spreadsheet behavior. */
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
      col = (col - 1 + cols) % cols
      break
    case "ArrowRight":
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

/**
 * `?` (Shift+/) opens the cheatsheet from anywhere on the planner — except
 * when an editable surface owns focus, where the keystroke means the literal
 * character. Returns true if the shortcut fired so the caller can preventDefault.
 */
export function isCheatsheetShortcut(e: KeyboardEvent): boolean {
  if (e.key !== "?") return false
  if (e.metaKey || e.ctrlKey || e.altKey) return false
  const target = e.target as Element | null
  if (!target) return true
  const tag = target.tagName
  if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return false
  if (target instanceof HTMLElement) {
    if (target.isContentEditable) return false
    // jsdom doesn't always wire isContentEditable from the attribute, so
    // fall back to the attribute itself for tests and old browsers.
    const attr = target.getAttribute("contenteditable")
    if (attr === "" || attr === "true") return false
  }
  return true
}
