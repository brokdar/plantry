import { useEffect, useState } from "react"

import { todayISO } from "@/lib/dates"

/** Ms until the next local midnight. */
function untilMidnight(): number {
  const now = new Date()
  const next = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1)
  return next.getTime() - now.getTime()
}

/**
 * The client's local date (YYYY-MM-DD) that moves on at midnight and when
 * the page becomes visible again, so a screen left open overnight (a
 * kitchen tablet on Today) doesn't keep showing yesterday.
 */
export function useTodayISO(): string {
  const [today, setToday] = useState(todayISO)

  useEffect(() => {
    let timer: ReturnType<typeof setTimeout>
    function sync() {
      clearTimeout(timer)
      setToday(todayISO())
      timer = setTimeout(sync, untilMidnight())
    }
    function onVisibility() {
      if (document.visibilityState === "visible") sync()
    }
    timer = setTimeout(sync, untilMidnight())
    document.addEventListener("visibilitychange", onVisibility)
    return () => {
      clearTimeout(timer)
      document.removeEventListener("visibilitychange", onVisibility)
    }
  }, [])

  return today
}
