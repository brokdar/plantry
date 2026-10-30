import type { PlanSlot } from "@/lib/api/plan"
import type { Session } from "@/lib/api/sessions"

/** One of the six batch palette entries (UX §3.1): 1 Rust … 6 Slate. */
export type BatchColour = 1 | 2 | 3 | 4 | 5 | 6

const PALETTE_SIZE = 6

/** The colour of the batch at 0-based position `index`, cycling after six. */
export function batchColour(index: number): BatchColour {
  return ((index % PALETTE_SIZE) + 1) as BatchColour
}

export interface ColouredBatch {
  session: Session
  colour: BatchColour
}

/**
 * Sorts sessions into cook-cell order (date, slot sort_order, slot id; the
 * session id breaks ties within one cell) and gives each its colour (D28).
 * One call per window feeds the board, the cook strip and every badge.
 */
export function colourBatches(
  sessions: readonly Session[],
  slots: readonly Pick<PlanSlot, "id" | "sort_order">[]
): ColouredBatch[] {
  const order = new Map(slots.map((s) => [s.id, s.sort_order]))
  const rank = (s: Session) => order.get(s.cook_slot_id) ?? Infinity
  return [...sessions]
    .sort(
      (a, b) =>
        a.cook_date.localeCompare(b.cook_date) ||
        rank(a) - rank(b) ||
        a.cook_slot_id - b.cook_slot_id ||
        a.id - b.id
    )
    .map((session, i) => ({ session, colour: batchColour(i) }))
}

/** Static class names per colour, spelled out so Tailwind generates them. */
export const BATCH_CLASSES: Record<
  BatchColour,
  { solid: string; outline: string; tint: string }
> = {
  1: {
    solid: "bg-batch-1 text-ground",
    outline: "border-batch-1 text-batch-1",
    tint: "bg-batch-1-tint",
  },
  2: {
    solid: "bg-batch-2 text-ground",
    outline: "border-batch-2 text-batch-2",
    tint: "bg-batch-2-tint",
  },
  3: {
    solid: "bg-batch-3 text-ground",
    outline: "border-batch-3 text-batch-3",
    tint: "bg-batch-3-tint",
  },
  4: {
    solid: "bg-batch-4 text-ground",
    outline: "border-batch-4 text-batch-4",
    tint: "bg-batch-4-tint",
  },
  5: {
    solid: "bg-batch-5 text-ground",
    outline: "border-batch-5 text-batch-5",
    tint: "bg-batch-5-tint",
  },
  6: {
    solid: "bg-batch-6 text-ground",
    outline: "border-batch-6 text-batch-6",
    tint: "bg-batch-6-tint",
  },
}

/** Outline for a cell while its batch is highlighted (`data-highlighted`,
 *  set by `useBatchCell`). It snaps on and off, so reduced motion needs no
 *  special case. */
export const BATCH_HIGHLIGHT_CLASS =
  "data-highlighted:outline-2 data-highlighted:outline-offset-2 data-highlighted:outline-ink"
