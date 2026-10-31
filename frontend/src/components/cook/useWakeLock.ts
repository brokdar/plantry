import { useEffect } from "react"

/**
 * Keeps the screen on while cook mode is open (UX §5.3): requests a screen
 * wake lock on mount, again when the page becomes visible (the browser
 * drops the lock when it is hidden), and releases it on unmount. A browser
 * without the API, or one that refuses, simply leaves the screen to its
 * own timeout.
 */
export function useWakeLock() {
  useEffect(() => {
    const wakeLock = (navigator as Partial<Navigator>).wakeLock
    if (!wakeLock) return

    let sentinel: WakeLockSentinel | null = null
    let mounted = true

    function request() {
      if (sentinel && !sentinel.released) return
      wakeLock!
        .request("screen")
        .then((s) => {
          if (mounted) sentinel = s
          else void s.release()
        })
        .catch(() => {
          // Refused (battery saver, hidden page): cooking goes on.
        })
    }
    function onVisibility() {
      if (document.visibilityState === "visible") request()
    }

    request()
    document.addEventListener("visibilitychange", onVisibility)
    return () => {
      mounted = false
      document.removeEventListener("visibilitychange", onVisibility)
      sentinel?.release().catch(() => {})
    }
  }, [])
}
