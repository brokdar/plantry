import { useSyncExternalStore } from "react"

/**
 * Debug workflow store — a localStorage-backed boolean that controls whether
 * the lookup pipeline returns its trace payload and the frontend surfaces the
 * debug panel. Mirrors the old Plantry `$debugWorkflow` Svelte store so users
 * can toggle it from Settings and have it apply anywhere a LookupPanel (or
 * future trace-aware surface) renders.
 *
 * Implementation: an in-memory value is the source of truth (so toggles work
 * even when localStorage is unavailable — incognito, tests, SSR). localStorage
 * is used for best-effort persistence across reloads and cross-tab sync.
 */

const STORAGE_KEY = "plantry_debug_workflow"
const EVENT_NAME = "plantry:debug-workflow"

function safeRead(): boolean {
  if (typeof window === "undefined") return false
  try {
    return window.localStorage.getItem(STORAGE_KEY) === "true"
  } catch {
    return false
  }
}

function safeWrite(value: boolean): void {
  if (typeof window === "undefined") return
  try {
    window.localStorage.setItem(STORAGE_KEY, String(value))
  } catch {
    // Private mode / quota / no-storage env — fine, in-memory value still flips.
  }
}

// Seed from localStorage if available; default false otherwise.
let currentValue = safeRead()

const listeners = new Set<() => void>()

function notify(): void {
  for (const listener of listeners) listener()
  if (typeof window !== "undefined") {
    window.dispatchEvent(new CustomEvent(EVENT_NAME))
  }
}

function writeValue(next: boolean) {
  if (next === currentValue) return
  currentValue = next
  safeWrite(next)
  notify()
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  // Cross-tab sync: a different tab's localStorage change fires a storage
  // event in this one; propagate it into our in-memory state.
  function onStorage(event: StorageEvent) {
    if (event.key && event.key !== STORAGE_KEY) return
    const fresh = safeRead()
    if (fresh !== currentValue) {
      currentValue = fresh
      listener()
    }
  }
  if (typeof window !== "undefined") {
    window.addEventListener("storage", onStorage)
  }
  return () => {
    listeners.delete(listener)
    if (typeof window !== "undefined") {
      window.removeEventListener("storage", onStorage)
    }
  }
}

function getSnapshot(): boolean {
  return currentValue
}

/** useDebugWorkflow returns the current flag and a setter. */
export function useDebugWorkflow(): [boolean, (next: boolean) => void] {
  const value = useSyncExternalStore(
    subscribe,
    getSnapshot,
    () => false // SSR snapshot
  )
  return [value, writeValue]
}

/** Imperative read for non-component code paths. */
export function getDebugWorkflow(): boolean {
  return currentValue
}
