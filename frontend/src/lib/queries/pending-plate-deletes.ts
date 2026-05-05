// Module-level registry of plates the user has optimistically deleted but
// whose backend DELETE is still queued behind a 5 s undo window. We need a
// shared registry (not a per-component ref) because *any* plate mutation
// invalidates `plateKeys.all` on settle and refetches; without flushing the
// queued deletes first, the server still has the plate and it reappears as
// an artifact. Plate mutation hooks call `flushPendingPlateDeletes()` from
// their `onMutate` so the queued deletes commit before the next mutation
// fires its refetch.

import type { Plate } from "@/lib/api/plates"

type PendingEntry = {
  timeoutId: ReturnType<typeof setTimeout>
  snapshot: Plate
  fire: () => Promise<void>
  promise: Promise<void> | null
}

const pending = new Map<number, PendingEntry>()

export const PENDING_DELETE_DELAY_MS = 5000

export function hasPendingPlateDelete(plateId: number): boolean {
  return pending.has(plateId)
}

export function getPendingPlateSnapshot(plateId: number): Plate | undefined {
  return pending.get(plateId)?.snapshot
}

export function registerPendingPlateDelete(
  plateId: number,
  snapshot: Plate,
  fire: () => Promise<void>
): void {
  cancelPendingPlateDelete(plateId)
  const timeoutId = setTimeout(() => {
    void runPending(plateId)
  }, PENDING_DELETE_DELAY_MS)
  pending.set(plateId, { timeoutId, snapshot, fire, promise: null })
}

export function cancelPendingPlateDelete(plateId: number): Plate | undefined {
  const entry = pending.get(plateId)
  if (!entry) return undefined
  clearTimeout(entry.timeoutId)
  pending.delete(plateId)
  return entry.snapshot
}

export async function flushPendingPlateDeletes(): Promise<void> {
  if (pending.size === 0) return
  const ids = [...pending.keys()]
  await Promise.all(ids.map((id) => runPending(id)))
}

async function runPending(plateId: number): Promise<void> {
  const entry = pending.get(plateId)
  if (!entry) return
  if (entry.promise) {
    await entry.promise
    return
  }
  clearTimeout(entry.timeoutId)
  const promise = entry.fire().finally(() => {
    pending.delete(plateId)
  })
  entry.promise = promise
  await promise
}

// Exposed for tests. Do not call from app code.
export function __resetPendingPlateDeletesForTests(): void {
  for (const entry of pending.values()) clearTimeout(entry.timeoutId)
  pending.clear()
}
