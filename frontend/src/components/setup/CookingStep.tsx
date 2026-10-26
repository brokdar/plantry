import { TriangleAlert } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Stepper, Switch } from "@/components/settings/controls"

import { SetupField } from "./SetupField"
import type { CookingDraft } from "./wizard-draft"

/** Leftovers past this many days get the food-safety note. */
const SAFE_KEEP_DAYS = 4

export function CookingStep({
  value,
  onChange,
}: {
  value: CookingDraft
  onChange: (next: CookingDraft) => void
}) {
  const { t } = useTranslation()
  const set = (patch: Partial<CookingDraft>) => onChange({ ...value, ...patch })
  const occasionsLabel = t("settings_page.cooking.occasions_label")
  const keepLabel = t("settings_page.cooking.keep_label")

  return (
    <div className="flex flex-col">
      <SetupField
        label={occasionsLabel}
        description={t("settings_page.cooking.occasions_effect", {
          count: value.occasions,
        })}
      >
        <Stepper
          label={occasionsLabel}
          value={value.occasions}
          min={1}
          max={3}
          onChange={(occasions) => set({ occasions })}
        />
      </SetupField>

      <SetupField
        label={t("settings_page.cooking.batch_label")}
        description={t(
          value.batch
            ? "settings_page.cooking.batch_effect_on"
            : "settings_page.cooking.batch_effect_off"
        )}
        htmlFor="setup-batch-cooking"
      >
        <Switch
          id="setup-batch-cooking"
          checked={value.batch}
          onChange={(batch) => set({ batch })}
        />
      </SetupField>

      <SetupField
        label={keepLabel}
        description={t("settings_page.cooking.keep_effect", {
          count: value.keepDays,
        })}
      >
        {value.keepDays > SAFE_KEEP_DAYS && (
          <p
            role="status"
            className="flex items-start gap-2 text-small text-ink"
            data-testid="setup-keep-days-warning"
          >
            <TriangleAlert
              className="mt-0.5 size-4 shrink-0 text-warning"
              strokeWidth={1.5}
              aria-hidden
            />
            {t("settings_page.cooking.keep_warning")}
          </p>
        )}
        <Stepper
          label={keepLabel}
          value={value.keepDays}
          min={1}
          max={7}
          format={(count) => t("settings_page.days", { count })}
          onChange={(keepDays) => set({ keepDays })}
        />
      </SetupField>

      <SetupField
        label={t("settings_page.cooking.freezing_label")}
        description={t(
          value.freezing
            ? "settings_page.cooking.freezing_effect_on"
            : "settings_page.cooking.freezing_effect_off"
        )}
        htmlFor="setup-freezing"
      >
        <Switch
          id="setup-freezing"
          checked={value.freezing}
          onChange={(freezing) => set({ freezing })}
        />
      </SetupField>
    </div>
  )
}
