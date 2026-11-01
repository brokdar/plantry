import { Check, Undo2 } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { changeSentence } from "@/lib/assistant/change-text"
import type { Change } from "@/lib/domain/assistantEvents"
import type { ChangeLineState } from "@/lib/stores/assistant-stream"

interface ChangeLineProps {
  change: Change
  /** null while the change can still be undone. */
  state: ChangeLineState | null
  onUndo: (id: number) => void
}

/** One applied agent write as a sentence with its Undo (D16). The button
 *  stays until the change is undone or later edits make a revert unsafe. */
export function ChangeLine({ change, state, onUndo }: ChangeLineProps) {
  const { t, i18n } = useTranslation()
  const sentence = changeSentence(t, i18n, change)
  return (
    <div
      data-testid="assistant-change"
      data-state={state ?? "applied"}
      className="flex items-center justify-between gap-3 rounded-cell border border-line bg-panel px-3 py-2 text-small text-ink"
    >
      <span className="flex min-w-0 items-center gap-2">
        <Check className="size-4 shrink-0 text-success" aria-hidden="true" />
        <span
          data-testid="assistant-change-text"
          className={state === "undone" ? "text-ink-muted line-through" : ""}
        >
          {sentence}
        </span>
      </span>
      {state === "undone" && (
        <span className="shrink-0 text-caption text-ink-muted">
          {t("assistant.undone")}
        </span>
      )}
      {state === "stale" && (
        <span className="shrink-0 text-caption text-ink-muted">
          {t("assistant.stale")}
        </span>
      )}
      {(state === null || state === "undoing") && change.undoable && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={state === "undoing"}
          onClick={() => onUndo(change.id)}
          data-testid="assistant-change-undo"
          aria-label={`${t("assistant.undo")}: ${sentence}`}
        >
          <Undo2 className="mr-1.5 size-3.5" aria-hidden="true" />
          {state === "undoing" ? t("assistant.undoing") : t("assistant.undo")}
        </Button>
      )}
    </div>
  )
}
