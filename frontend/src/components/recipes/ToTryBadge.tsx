import { useTranslation } from "react-i18next"

import { useSettingItems } from "@/lib/queries/settings"

/** The "To try" badge of a recipe the Chef added and the household has not
 *  opened yet (NEW-3); opening the recipe clears it. Shown only while the
 *  Chef is on. */
export function ToTryBadge({ className = "" }: { className?: string }) {
  const { t } = useTranslation()
  const chefOn = useSettingItems().data?.["chef.enabled"]?.value === "true"
  if (!chefOn) return null
  return (
    <span
      data-testid="to-try-badge"
      className={`inline-flex items-center rounded-full bg-batch-3-tint px-2 py-0.5 text-small font-medium text-batch-3 ${className}`}
    >
      {t("recipes.to_try")}
    </span>
  )
}
