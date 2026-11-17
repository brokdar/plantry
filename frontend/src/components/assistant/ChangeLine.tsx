import { Check, Undo2 } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { changeSentence, hiddenOpCount } from "@/lib/assistant/change-text"
import type { Change } from "@/lib/domain/assistantEvents"
import type { ChangeLineState } from "@/lib/stores/assistant-stream"

interface ChangeLineProps {
  change: Change
  /** null while the change can still be undone. */
  state: ChangeLineState | null
  onUndo: (id: number) => void
  /** Persisted turns caption "Undone" once per turn instead of per line. */
  hideUndoneLabel?: boolean
}

/** One applied agent write as a sentence with its Undo (D16). The button
 *  stays until the change is undone or later edits make a revert unsafe.
 *  A change of more than three ops shows the first three and a "+N more"
 *  toggle that expands the full list. */
export function ChangeLine({
  change,
  state,
  onUndo,
  hideUndoneLabel,
}: ChangeLineProps) {
  const { t, i18n } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const hidden = hiddenOpCount(change)
  const sentence = changeSentence(t, i18n, change, { allOps: true })
  const shown =
    hidden > 0
      ? changeSentence(t, i18n, change, { allOps: expanded, omitMore: true })
      : sentence
  const blocked =
    state === "confirmed" ||
    (state === null && change.undo_blocked === "confirmed")
  return (
    <div
      data-testid="assistant-change"
      data-state={state ?? "applied"}
      className="flex items-center justify-between gap-3 rounded-cell border border-line bg-panel px-3 py-2 text-small text-ink"
    >
      <span className="flex min-w-0 items-center gap-2">
        <Check className="size-4 shrink-0 text-success" aria-hidden="true" />
        <span>
          <span
            data-testid="assistant-change-text"
            className={state === "undone" ? "text-ink-muted line-through" : ""}
          >
            {shown}
          </span>
          {hidden > 0 && (
            <button
              type="button"
              data-testid="assistant-change-more"
              aria-expanded={expanded}
              onClick={() => setExpanded((v) => !v)}
              className="ml-1.5 rounded-sm text-caption text-ink-muted underline underline-offset-2 hover:text-ink focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
            >
              {expanded
                ? t("assistant.change.show_less")
                : t("assistant.change.op_more", { count: hidden })}
            </button>
          )}
        </span>
      </span>
      {state === "undone" && !hideUndoneLabel && (
        <span className="shrink-0 text-caption text-ink-muted">
          {t("assistant.undone")}
        </span>
      )}
      {blocked && (
        <span
          data-testid="assistant-change-blocked"
          className="max-w-[45%] text-right text-caption text-ink-muted"
        >
          {t("assistant.change.blocked_confirmed")}
        </span>
      )}
      {state === "stale" && (
        <span
          data-testid="assistant-change-stale"
          className="max-w-[45%] text-right text-caption text-ink"
        >
          {t("assistant.stale")}
        </span>
      )}
      {state === "nothing_to_undo" && (
        <span
          data-testid="assistant-change-nothing"
          className="max-w-[45%] text-right text-caption text-ink-muted"
        >
          {t("assistant.nothing_to_undo")}
        </span>
      )}
      {!change.recorded && (
        <span
          data-testid="assistant-change-unrecorded"
          className="max-w-[45%] text-right text-caption text-ink"
        >
          {t("assistant.not_recorded")}
        </span>
      )}
      {!blocked &&
        (state === null || state === "undoing") &&
        change.undoable && (
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
