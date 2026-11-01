import { Loader2 } from "lucide-react"
import { useTranslation } from "react-i18next"

interface StatusLineProps {
  streaming: boolean
  /** Name of the tool executing right now, or null. */
  status: string | null
}

/** One live line under the transcript: what the assistant is doing. */
export function StatusLine({ streaming, status }: StatusLineProps) {
  const { t } = useTranslation()
  return (
    <div
      role="status"
      aria-live="polite"
      data-testid="assistant-status"
      className="min-h-5 px-4 text-small text-ink-muted"
    >
      {streaming && (
        <span className="inline-flex items-center gap-2">
          <Loader2 className="size-4 animate-spin" aria-hidden="true" />
          {status
            ? t("assistant.status.tool", { name: status })
            : t("assistant.status.thinking")}
        </span>
      )}
    </div>
  )
}
