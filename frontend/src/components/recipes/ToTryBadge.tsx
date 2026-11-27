import { useTranslation } from "react-i18next"

/** The "To try" badge of a recipe the Chef added and the household has not
 *  opened yet (NEW-3); opening the recipe clears it. */
export function ToTryBadge({ className = "" }: { className?: string }) {
  const { t } = useTranslation()
  return (
    <span
      data-testid="to-try-badge"
      className={`inline-flex items-center rounded-full bg-batch-3-tint px-2 py-0.5 text-small font-medium text-batch-3 ${className}`}
    >
      {t("recipes.to_try")}
    </span>
  )
}
