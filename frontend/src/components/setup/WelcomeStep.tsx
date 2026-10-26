import { useTranslation } from "react-i18next"

import { OptionCards } from "@/components/settings/controls"

import type { Language } from "./wizard-draft"

const LANGUAGES: { value: Language; labelKey: string }[] = [
  { value: "en", labelKey: "display_preferences.language_english" },
  { value: "de", labelKey: "display_preferences.language_german" },
]

export function WelcomeStep({
  value,
  onChange,
}: {
  value: Language
  onChange: (next: Language) => void
}) {
  const { t } = useTranslation()
  return (
    <OptionCards
      name="language"
      legend={t("display_preferences.language_label")}
      value={value}
      columns={2}
      onChange={onChange}
      options={LANGUAGES.map(({ value, labelKey }) => ({
        value,
        label: t(labelKey),
      }))}
    />
  )
}
