import { Languages } from "lucide-react"
import { useTranslation } from "react-i18next"

import { SettingsCard } from "@/components/editorial/SettingsCard"
import { profileKeys } from "@/lib/queries/keys"
import { useProfile, useUpdateProfile } from "@/lib/queries/profile"
import { queryClient } from "@/lib/query-client"
import { cn } from "@/lib/utils"

const OPTIONS = [
  { value: "en", labelKey: "display_preferences.language_english" },
  { value: "de", labelKey: "display_preferences.language_german" },
] as const

export function LanguagePreference() {
  const { t, i18n } = useTranslation()
  const { data: profile } = useProfile()
  const updateMut = useUpdateProfile()

  // Use i18n.language as the source of truth for the highlighted pill so the
  // UI reflects the pick instantly, before the profile mutation resolves.
  const active = (i18n.language || profile?.locale || "en").split("-")[0]

  async function select(value: string) {
    if (value === active) return
    await i18n.changeLanguage(value)
    if (!profile) return
    // Optimistically reflect the new locale in the profile cache so other
    // subscribers (AppShell's i18n sync, ProfileEditor) stay coherent while
    // the mutation is in flight.
    queryClient.setQueryData(profileKeys.detail, {
      ...profile,
      locale: value,
    })
    // Merge the full profile — /api/profile is a full-replace PUT.
    await updateMut.mutateAsync({
      kcal_target: profile.kcal_target,
      protein_pct: profile.protein_pct,
      fat_pct: profile.fat_pct,
      carbs_pct: profile.carbs_pct,
      dietary_restrictions: profile.dietary_restrictions,
      preferences: profile.preferences,
      system_prompt: profile.system_prompt,
      locale: value,
    })
  }

  return (
    <SettingsCard
      title={t("display_preferences.language_label")}
      icon={<Languages className="size-5" aria-hidden />}
    >
      <div className="space-y-4">
        <p className="text-xs text-on-surface-variant">
          {t("display_preferences.language_description")}
        </p>
        <div
          role="radiogroup"
          aria-label={t("display_preferences.language_label")}
          className="inline-flex rounded-full bg-surface-container-low p-1"
        >
          {OPTIONS.map(({ value, labelKey }) => {
            const isActive = active === value
            return (
              <button
                key={value}
                type="button"
                role="radio"
                aria-checked={isActive}
                onClick={() => select(value)}
                data-testid={`language-option-${value}`}
                className={cn(
                  "rounded-full px-4 py-2 text-xs font-semibold transition-all",
                  isActive
                    ? "editorial-shadow bg-surface-container-lowest text-primary"
                    : "text-on-surface-variant hover:text-on-surface"
                )}
              >
                {t(labelKey)}
              </button>
            )
          })}
        </div>
      </div>
    </SettingsCard>
  )
}
