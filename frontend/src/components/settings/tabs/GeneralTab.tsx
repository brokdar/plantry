import { DisplayPreferences } from "@/components/settings/DisplayPreferences"
import { LanguagePreference } from "@/components/settings/LanguagePreference"

export function GeneralTab() {
  return (
    <div className="space-y-6">
      <DisplayPreferences />
      <LanguagePreference />
    </div>
  )
}
