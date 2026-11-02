import { useNavigate } from "@tanstack/react-router"
import { X } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { useIsPhone } from "@/components/plan/use-is-phone"
import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetTitle,
} from "@/components/ui/sheet"
import type { RejectReason } from "@/lib/api/discovery"
import { discoveryAvailable } from "@/lib/discovery/available"
import { useLocalStorageState } from "@/lib/hooks/useLocalStorageState"
import {
  useAnotherSuggestion,
  useDailySuggestion,
  useRejectSuggestion,
} from "@/lib/queries/discovery"
import { useSettingItems } from "@/lib/queries/settings"
import { cn } from "@/lib/utils"

import { RejectReasons } from "./RejectReasons"
import { SuggestionCard } from "./SuggestionCard"

/** localStorage: the date the sheet was last closed on. */
export const DISMISSED_KEY = "plantry.discovery.dismissed"

/**
 * The daily idea over Today (UX §5.8). Asks for it only when discovery is
 * available (a provider is set) and `discovery.daily_enabled` is on, and not
 * again on a day it was closed. Nothing shows while it loads, on 204 or when
 * the run failed (UX §5.9). Closing only remembers the day; "Not for me"
 * is the one thing that rejects.
 */
export function DailySuggestionSheet({ date }: { date: string }) {
  const { t } = useTranslation()
  const phone = useIsPhone()
  const navigate = useNavigate()
  const settings = useSettingItems()
  const [dismissed, setDismissed] = useLocalStorageState<string>(
    DISMISSED_KEY,
    ""
  )
  const enabled =
    discoveryAvailable(settings.data) &&
    settings.data?.["discovery.daily_enabled"]?.value !== "false" &&
    dismissed !== date
  const daily = useDailySuggestion(date, enabled)
  const another = useAnotherSuggestion()
  const reject = useRejectSuggestion()
  const [rejecting, setRejecting] = useState(false)

  const data = enabled ? daily.data : undefined
  const suggestion = data?.kind === "suggestion" ? data.suggestion : undefined
  // After "Show another" the sheet stays open while the next idea is found.
  const open =
    Boolean(suggestion) || (!another.isIdle && data?.kind === "running")
  if (!enabled || !open) return null

  const close = () => setDismissed(date)

  function onReject(reason: RejectReason | undefined) {
    if (!suggestion) return
    // Close first: the dismissal disables the daily query before the
    // reject invalidates it, so no refetch can start a paid run.
    close()
    reject.mutate({ id: suggestion.id, reason })
  }

  return (
    <Sheet open onOpenChange={(next) => !next && close()}>
      <SheetContent
        side={phone ? "bottom" : "right"}
        className={cn(
          "flex flex-col gap-0 bg-panel p-0 text-ink shadow-sheet",
          phone
            ? "max-h-[90dvh] rounded-t-sheet"
            : "w-full sm:max-w-md md:max-w-md"
        )}
        data-testid="daily-suggestion-sheet"
        showCloseButton={false}
      >
        <div className="flex items-start justify-between gap-2 p-4 pb-2">
          <SheetTitle className="font-heading text-h3 text-ink">
            {t("discovery.sheet.title")}
          </SheetTitle>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t("discovery.sheet.close")}
            onClick={close}
          >
            <X />
          </Button>
        </div>
        <SheetDescription className="sr-only">
          {suggestion?.name ?? t("discovery.sheet.finding_another")}
        </SheetDescription>
        <div className="flex-1 overflow-y-auto px-4 pb-4">
          {suggestion && data?.kind === "suggestion" ? (
            <SuggestionCard suggestion={suggestion} />
          ) : (
            <p role="status" className="text-body text-ink-muted">
              {t("discovery.sheet.finding_another")}
            </p>
          )}
        </div>
        {suggestion && (
          <div className="flex flex-col gap-3 border-t border-line p-4">
            {rejecting ? (
              <RejectReasons onPick={onReject} disabled={reject.isPending} />
            ) : (
              <div className="flex flex-wrap gap-2">
                <Button
                  className="h-11 px-4 md:pointer-fine:h-9"
                  onClick={() => {
                    close()
                    void navigate({
                      to: "/recipes",
                      search: { add: "discover", suggestion: suggestion.id },
                    })
                  }}
                >
                  {t("discovery.sheet.add")}
                </Button>
                <Button
                  variant="outline"
                  className="h-11 px-4 md:pointer-fine:h-9"
                  onClick={() => setRejecting(true)}
                >
                  {t("discovery.sheet.not_for_me")}
                </Button>
                <Button
                  variant="ghost"
                  className="h-11 px-4 md:pointer-fine:h-9"
                  disabled={another.isPending}
                  onClick={() => another.mutate(date)}
                >
                  {t("discovery.sheet.another")}
                </Button>
              </div>
            )}
          </div>
        )}
      </SheetContent>
    </Sheet>
  )
}
