import { useTranslation } from "react-i18next"

import { usePatchProfile, useProfile } from "@/lib/queries/profile"

import { OptionCards } from "./controls"
import { useSavedNote } from "./setting-field"
import { SettingRow } from "./SettingRow"

const OPTIONS = [
  { value: "en", labelKey: "display_preferences.language_english" },
  { value: "de", labelKey: "display_preferences.language_german" },
] as const

type Language = (typeof OPTIONS)[number]["value"]

/** Interface language, stored as `profile.locale`. */
export function LanguagePreference() {
  const { t, i18n } = useTranslation()
  const { data: profile } = useProfile()
  const patch = usePatchProfile()
  const { saved, markSaved } = useSavedNote()

  // i18n.language is the source of truth for the selection so the UI
  // reflects the pick instantly, before the profile save resolves.
  const active = (i18n.language || profile?.locale || "en").split(
    "-"
  )[0] as Language
  const activeOption = OPTIONS.find((o) => o.value === active) ?? OPTIONS[0]

  async function select(value: Language) {
    if (value === active) return
    await i18n.changeLanguage(value)
    if (!profile) return
    patch.mutate({ locale: value }, { onSuccess: markSaved })
  }

  return (
    <SettingRow
      label={t("display_preferences.language_label")}
      description={t("settings_page.system_section.language_effect", {
        language: t(activeOption.labelKey),
      })}
      saved={saved}
    >
      <OptionCards
        name="language"
        legend={t("display_preferences.language_label")}
        value={active}
        columns={2}
        onChange={select}
        options={OPTIONS.map(({ value, labelKey }) => ({
          value,
          label: t(labelKey),
        }))}
      />
    </SettingRow>
  )
}
