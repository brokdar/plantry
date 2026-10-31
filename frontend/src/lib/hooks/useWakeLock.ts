import { useEffect } from "react"

/**
 * Keeps the screen on while `active` (cook mode, UX §5.3; Shop's in-store
 * mode): requests a screen wake lock, again when the page becomes visible
 * (the browser drops the lock when it is hidden), and releases it when
 * `active` turns off or on unmount. A browser without the API, or one that
 * refuses, simply leaves the screen to its own timeout.
 */
export function useWakeLock(active = true) {
  useEffect(() => {
    const wakeLock = (navigator as Partial<Navigator>).wakeLock
    if (!active || !wakeLock) return

    let sentinel: WakeLockSentinel | null = null
    let mounted = true
    let pending = false

    function request() {
      if (pending || (sentinel && !sentinel.released)) return
      pending = true
      wakeLock!
        .request("screen")
        .then((s) => {
          if (mounted) sentinel = s
          else void s.release()
        })
        .catch(() => {
          // Refused (battery saver, hidden page): cooking goes on.
        })
        .finally(() => {
          pending = false
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
  }, [active])
}
