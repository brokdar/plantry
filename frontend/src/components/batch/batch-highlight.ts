import { createContext, use, useCallback } from "react"

/** How long a linked highlight stays on (UX §4). */
export const HIGHLIGHT_MS = 2000

export interface BatchHighlightActions {
  /** Outlines every cell of the session for HIGHLIGHT_MS. */
  highlight: (sessionId: number) => void
  /** Scrolls the session's cook cell into view, then highlights the batch. */
  reveal: (sessionId: number) => void
  /** Registers `el` as the cook cell of `sessionId`; returns the cleanup. */
  registerCookCell: (sessionId: number, el: HTMLElement) => () => void
}

const noop: BatchHighlightActions = {
  highlight: () => {},
  reveal: () => {},
  registerCookCell: () => () => {},
}

// State and actions are separate so badges (actions only) do not re-render
// when the highlight changes. Without a provider everything works and
// nothing highlights.
export const HighlightedContext = createContext<number | null>(null)
export const ActionsContext = createContext<BatchHighlightActions>(noop)

/** The highlight actions, for badges and the cook strip. */
export function useBatchHighlight(): BatchHighlightActions {
  return use(ActionsContext)
}

/**
 * Props for a plan cell that shows batches: spread them on the cell element.
 * `sessionIds` are the sessions the cell eats from; `cookOf` the sessions
 * cooked in it (the cook strip scrolls to those). While one of its sessions
 * is highlighted the cell carries `data-highlighted` (style it with
 * `BATCH_HIGHLIGHT_CLASS`).
 */
export function useBatchCell(
  sessionIds: readonly number[],
  cookOf: readonly number[] = []
) {
  const highlighted = use(HighlightedContext)
  const { registerCookCell } = use(ActionsContext)
  const cookKey = cookOf.join(",")

  const ref = useCallback(
    (el: HTMLElement | null) => {
      if (!el || !cookKey) return
      const cleanups = cookKey
        .split(",")
        .map((id) => registerCookCell(Number(id), el))
      return () => cleanups.forEach((cleanup) => cleanup())
    },
    [cookKey, registerCookCell]
  )

  return {
    ref,
    "data-highlighted":
      highlighted !== null && sessionIds.includes(highlighted)
        ? true
        : undefined,
  }
}
