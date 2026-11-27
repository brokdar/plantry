import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { useAssistantDid, useUndoPlacedFind } from "@/lib/queries/assistant-did"

/**
 * "What the assistant did" in a window (PRO-5): one line per change it made
 * without being asked, each with Undo. Nothing shows while the list is
 * empty or the Chef is off.
 */
export function AssistantDid({
  start,
  enabled,
}: {
  start: string
  enabled: boolean
}) {
  const { t } = useTranslation()
  const list = useAssistantDid(start, enabled)
  const undo = useUndoPlacedFind()
  const items = enabled ? (list.data?.items ?? []) : []
  if (items.length === 0) return null
  const more = list.data?.more ?? 0
  return (
    <section
      aria-labelledby="assistant-did-title"
      data-testid="assistant-did"
      className="rounded-card flex flex-col gap-2 border border-line bg-panel p-3"
    >
      <h2 id="assistant-did-title" className="text-small font-medium text-ink">
        {t("plan.assistant_did.title")}
      </h2>
      <ul className="flex flex-col gap-1">
        {items.map((item) => (
          <li
            key={`${item.kind}-${item.find_id}`}
            className="flex items-center justify-between gap-3 text-body text-ink"
          >
            <span>
              {t("plan.assistant_did.find_placed", { name: item.name })}
            </span>
            <Button
              variant="ghost"
              size="sm"
              disabled={undo.isPending}
              aria-label={t("plan.assistant_did.undo_label", {
                name: item.name,
              })}
              onClick={() => undo.mutate(item.find_id)}
            >
              {t("plan.assistant_did.undo")}
            </Button>
          </li>
        ))}
      </ul>
      {more > 0 && (
        <p className="text-small text-ink-muted">
          {t("plan.assistant_did.more", { count: more })}
        </p>
      )}
    </section>
  )
}
