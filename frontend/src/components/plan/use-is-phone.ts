import { useSyncExternalStore } from "react"

const PHONE_QUERY = "(max-width: 767px)"

function subscribe(onChange: () => void) {
  if (typeof window.matchMedia !== "function") return () => {}
  const mq = window.matchMedia(PHONE_QUERY)
  mq.addEventListener("change", onChange)
  return () => mq.removeEventListener("change", onChange)
}

function isPhone() {
  return (
    typeof window.matchMedia === "function" &&
    window.matchMedia(PHONE_QUERY).matches
  )
}

/** Below 768 px: the board's day list and the bottom sheet. */
export function useIsPhone() {
  return useSyncExternalStore(subscribe, isPhone)
}
