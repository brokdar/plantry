import { onlineManager } from "@tanstack/react-query"
import { WifiOff } from "lucide-react"
import { useSyncExternalStore } from "react"
import { useTranslation } from "react-i18next"

/**
 * Says so when the browser is offline. Plantry is online-only (the server
 * owns the data), so there is no queue: reads wait for the connection and
 * saves fail at once with the network toast. The live region is always
 * mounted so the message is announced when it appears.
 */
export function OfflineBanner() {
  const { t } = useTranslation()
  const online = useSyncExternalStore(
    (notify) => onlineManager.subscribe(notify),
    () => onlineManager.isOnline()
  )
  return (
    <div role="status" data-testid="offline-banner">
      {!online && (
        <p className="flex items-center gap-2 border-b border-line bg-panel-sunken px-4 py-2 text-small text-ink">
          <WifiOff className="size-4 shrink-0" aria-hidden />
          {t("nav.offline")}
        </p>
      )}
    </div>
  )
}
