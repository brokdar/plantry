import { useTranslation } from "react-i18next"

import { SettingsCard } from "@/components/editorial/SettingsCard"
import { ToggleRow } from "@/components/editorial/ToggleRow"
import { useDebugWorkflow } from "@/lib/debugWorkflow"

/**
 * DeveloperPreferences exposes workflow debug mode — a local (per-browser)
 * flag that surfaces the lookup pipeline trace inside the ingredient editor.
 * The value is stored in localStorage and kept in sync across tabs.
 */
export function DeveloperPreferences() {
  const { t } = useTranslation()
  const [debug, setDebug] = useDebugWorkflow()

  return (
    <SettingsCard title={t("developer_preferences.title")}>
      <ToggleRow
        title={t("developer_preferences.debug_title")}
        description={t("developer_preferences.debug_description")}
        checked={debug}
        onChange={setDebug}
        testId="debug-workflow-toggle"
      />
    </SettingsCard>
  )
}
