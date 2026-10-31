import { useCallback, useSyncExternalStore } from "react"

const subscribers = new Map<string, Set<() => void>>()

function subscribe(key: string, callback: () => void) {
  let listeners = subscribers.get(key)
  if (!listeners) {
    listeners = new Set()
    subscribers.set(key, listeners)
  }
  listeners.add(callback)

  function onStorage(e: StorageEvent) {
    if (e.key === key) callback()
  }
  if (typeof window !== "undefined") {
    window.addEventListener("storage", onStorage)
  }

  return () => {
    listeners?.delete(callback)
    if (typeof window !== "undefined") {
      window.removeEventListener("storage", onStorage)
    }
  }
}

function readStorage(key: string): string | null {
  if (typeof window === "undefined") return null
  try {
    return window.localStorage?.getItem(key) ?? null
  } catch {
    return null
  }
}

function writeStorage(key: string, value: string) {
  if (typeof window === "undefined") return
  try {
    window.localStorage?.setItem(key, value)
  } catch {
    // ignore storage errors (private mode, quota, etc.)
  }
}

function notify(key: string) {
  subscribers.get(key)?.forEach((cb) => cb())
}

export function useLocalStorageState<T extends string>(
  key: string,
  defaultValue: T,
  isValid?: (value: string) => value is T
): [T, (value: T) => void] {
  const value = useSyncExternalStore(
    useCallback((cb) => subscribe(key, cb), [key]),
    () => {
      const raw = readStorage(key)
      if (raw == null) return defaultValue
      if (isValid && !isValid(raw)) return defaultValue
      return raw as T
    },
    () => defaultValue
  )

  const setValue = useCallback(
    (next: T) => {
      writeStorage(key, next)
      notify(key)
    },
    [key]
  )

  return [value, setValue]
}
