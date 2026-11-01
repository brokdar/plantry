import { Loader2 } from "lucide-react"
import { useTranslation } from "react-i18next"

import { statusKey } from "@/lib/assistant/change-text"

interface StatusLineProps {
  streaming: boolean
  /** Name of the tool executing right now, or null. */
  status: string | null
}

/** One muted line under the transcript: what the assistant is doing, in
 *  words ("Reading your plan…"), never a tool name. */
export function StatusLine({ streaming, status }: StatusLineProps) {
  const { t } = useTranslation()
  return (
    <div
      data-testid="assistant-status"
      className="min-h-5 px-4 text-small text-ink-muted"
    >
      {streaming && (
        <span className="inline-flex items-center gap-2">
          <Loader2 className="size-4 animate-spin" aria-hidden="true" />
          {status ? t(statusKey(status)) : t("assistant.status.thinking")}
        </span>
      )}
    </div>
  )
}
