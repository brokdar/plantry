import { useTranslation } from "react-i18next"

import { useTheme } from "@/components/theme-provider"

import { OptionCards } from "./controls"
import { useSavedNote } from "./setting-field"
import { SettingRow } from "./SettingRow"

const THEMES = ["light", "dark", "system"] as const

/** Theme: light, dark or follow the device. Stored in this browser. */
export function DisplayPreferences() {
  const { t } = useTranslation()
  const { theme, setTheme } = useTheme()
  const { saved, markSaved } = useSavedNote()

  return (
    <SettingRow
      label={t("display_preferences.theme_label")}
      description={t(`settings_page.system_section.theme_effect.${theme}`)}
      saved={saved}
    >
      <OptionCards
        name="theme"
        legend={t("display_preferences.theme_label")}
        value={theme}
        columns={3}
        onChange={(next) => {
          setTheme(next)
          markSaved()
        }}
        options={THEMES.map((value) => ({
          value,
          label: t(`display_preferences.theme_${value}`),
        }))}
      />
    </SettingRow>
  )
}
