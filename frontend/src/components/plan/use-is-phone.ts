import { useSyncExternalStore } from "react"

const PHONE_QUERY = "(max-width: 767px)"
/** The width from which the assistant panel docks beside the board. */
const WIDE_QUERY = "(min-width: 1280px)"

function watch(query: string) {
  return {
    subscribe(onChange: () => void) {
      if (typeof window.matchMedia !== "function") return () => {}
      const mq = window.matchMedia(query)
      mq.addEventListener("change", onChange)
      return () => mq.removeEventListener("change", onChange)
    },
    get() {
      return (
        typeof window.matchMedia === "function" &&
        window.matchMedia(query).matches
      )
    },
  }
}

const phone = watch(PHONE_QUERY)
const wide = watch(WIDE_QUERY)

/** Below 768 px: the board's day list and the bottom sheet. */
export function useIsPhone() {
  return useSyncExternalStore(phone.subscribe, phone.get)
}

/** From 1280 px: room for the assistant panel beside the board. */
export function useIsWide() {
  return useSyncExternalStore(wide.subscribe, wide.get)
}
