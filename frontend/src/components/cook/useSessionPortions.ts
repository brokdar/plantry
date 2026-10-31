import { useEffect, useRef, useState } from "react"

import type { Session } from "@/lib/api/sessions"
import { useUpdateSession } from "@/lib/queries/sessions"

/** How long the stepper waits for more taps before it writes. */
export const PORTIONS_DEBOUNCE_MS = 400

/**
 * Cook mode's session portions (UX §5.3): the stepper shows the new value
 * at once and writes `PUT /api/cook-sessions/{id}` once the taps pause for
 * 400 ms, so 6 → 8 is one request. Frozen portions are clamped to the new
 * total; the note is kept.
 *
 * - **Last write wins:** session updates share one mutation scope, so they
 *   reach the server in order, and the shown value only falls back to the
 *   server's once the newest write has settled.
 * - **Leaving mid-debounce flushes:** an edit the user made is written even
 *   if they leave (or tap Done cooking) within the 400 ms. `flush` does it
 *   on demand.
 */
export function useSessionPortions(session: Session | undefined) {
  const update = useUpdateSession()
  const [draft, setDraft] = useState<number | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const queued = useRef<number | null>(null)
  const seq = useRef(0)
  const latest = useRef({ session, mutate: update.mutate })
  useEffect(() => {
    latest.current = { session, mutate: update.mutate }
  })

  function flush() {
    clearTimeout(timer.current)
    timer.current = undefined
    const portions = queued.current
    const { session: s, mutate } = latest.current
    if (portions === null || !s) return
    queued.current = null
    const mine = ++seq.current
    mutate(
      {
        id: s.id,
        input: {
          portions,
          frozen_portions: Math.min(s.frozen_portions, portions),
          note: s.note,
        },
      },
      {
        onSettled: () => {
          if (seq.current === mine) setDraft(null)
        },
      }
    )
  }

  const flushRef = useRef(flush)
  useEffect(() => {
    flushRef.current = flush
  })
  useEffect(() => () => flushRef.current(), [])

  function change(portions: number) {
    setDraft(portions)
    queued.current = portions
    clearTimeout(timer.current)
    timer.current = setTimeout(flush, PORTIONS_DEBOUNCE_MS)
  }

  return {
    portions: draft ?? session?.portions ?? 0,
    change,
    flush,
  }
}
