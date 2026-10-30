import { useEffect, useMemo, useRef, useState, type ReactNode } from "react"

import {
  ActionsContext,
  HIGHLIGHT_MS,
  HighlightedContext,
  type BatchHighlightActions,
  type Highlight,
} from "./batch-highlight"

/**
 * Owns the linked batch highlight for a board or list (UX §4). Badges call
 * `highlight` through `useBatchHighlight`; cells read it through
 * `useBatchCell`. One highlight at a time, for HIGHLIGHT_MS.
 */
export function BatchHighlightProvider({ children }: { children: ReactNode }) {
  const [highlighted, setHighlighted] = useState<Highlight | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const cookCells = useRef(new Map<number, HTMLElement>())

  useEffect(() => () => clearTimeout(timer.current), [])

  const actions = useMemo<BatchHighlightActions>(() => {
    const highlight: BatchHighlightActions["highlight"] = (
      sessionId,
      colour
    ) => {
      clearTimeout(timer.current)
      setHighlighted({ sessionId, colour })
      timer.current = setTimeout(() => setHighlighted(null), HIGHLIGHT_MS)
    }
    return {
      highlight,
      reveal(sessionId, colour) {
        const reduce = window.matchMedia?.(
          "(prefers-reduced-motion: reduce)"
        ).matches
        cookCells.current.get(sessionId)?.scrollIntoView({
          block: "nearest",
          inline: "center",
          behavior: reduce ? "auto" : "smooth",
        })
        highlight(sessionId, colour)
      },
      registerCookCell(sessionId, el) {
        cookCells.current.set(sessionId, el)
        return () => {
          if (cookCells.current.get(sessionId) === el) {
            cookCells.current.delete(sessionId)
          }
        }
      },
    }
  }, [])

  return (
    <ActionsContext value={actions}>
      <HighlightedContext value={highlighted}>{children}</HighlightedContext>
    </ActionsContext>
  )
}
