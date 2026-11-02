import { useTranslation } from "react-i18next"

/** Explains the accent "new" marker on ingredients. */
export function NewLegend() {
  const { t } = useTranslation()
  return (
    <p className="text-small text-ink-muted" data-testid="discovery-legend">
      {t("discovery.card.legend")}
    </p>
  )
}
