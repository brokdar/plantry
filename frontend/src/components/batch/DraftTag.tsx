import { useTranslation } from "react-i18next"

/** "Draft" next to a plate that is not accepted yet (dashed, like a draft
 *  cell on the board). */
export function DraftTag() {
  const { t } = useTranslation()
  return (
    <span
      className="shrink-0 rounded-tag border border-dashed border-ink-muted px-1.5 text-caption text-ink-muted"
      data-testid="draft-tag"
    >
      {t("batch.draft")}
    </span>
  )
}
