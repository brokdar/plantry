import { useTranslation } from "react-i18next"

import { AIProviderControls } from "@/components/settings/AIProviderControls"
import { Switch } from "@/components/settings/controls"
import { Textarea } from "@/components/ui/textarea"
import { useSettingItems } from "@/lib/queries/settings"

import { SetupField } from "./SetupField"
import type { AssistantDraft } from "./wizard-draft"

/** Provider, key and model save themselves (the Settings controls); the
 *  notes and the daily suggestion are saved on Next. */
export function AssistantStep({
  value,
  onChange,
}: {
  value: AssistantDraft
  onChange: (next: AssistantDraft) => void
}) {
  const { t } = useTranslation()
  const { data: items } = useSettingItems()
  const key = items?.["ai.api_key"]
  const hasKey = key?.source === "db" || key?.source === "env"
  const set = (patch: Partial<AssistantDraft>) =>
    onChange({ ...value, ...patch })

  return (
    <div className="flex flex-col">
      {!hasKey && (
        <p
          className="rounded-lg border border-line bg-panel px-4 py-3 text-body text-ink"
          data-testid="setup-assistant-later"
        >
          {t("setup.assistant.later")}
        </p>
      )}
      <div className="border-b border-line py-5">
        <AIProviderControls />
      </div>

      <SetupField
        label={t("settings_page.assistant.notes_label")}
        description={t("settings_page.assistant.notes_effect")}
        htmlFor="setup-assistant-notes"
      >
        <Textarea
          id="setup-assistant-notes"
          rows={4}
          placeholder={t("settings_page.assistant.notes_placeholder")}
          value={value.notes}
          onChange={(e) => set({ notes: e.target.value })}
        />
      </SetupField>

      <SetupField
        label={t("settings_page.assistant.daily_label")}
        description={t(
          value.daily
            ? "settings_page.assistant.daily_effect_on"
            : "settings_page.assistant.daily_effect_off"
        )}
        htmlFor="setup-daily-suggestion"
      >
        <Switch
          id="setup-daily-suggestion"
          checked={value.daily}
          onChange={(daily) => set({ daily })}
        />
      </SetupField>
    </div>
  )
}
