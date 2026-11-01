import { useTranslation } from "react-i18next"

import { changeSentence } from "@/lib/assistant/change-text"
import type { Change } from "@/lib/domain/assistantEvents"

/** Announces each applied change politely to screen readers (UX §8): the
 *  newest change's sentence, visually hidden. */
export function LiveRegion({ change }: { change: Change | null }) {
  const { t, i18n } = useTranslation()
  return (
    <div
      role="status"
      aria-live="polite"
      aria-label={t("assistant.live_label")}
      data-testid="assistant-live"
      className="sr-only"
    >
      {change ? changeSentence(t, i18n, change) : ""}
    </div>
  )
}
