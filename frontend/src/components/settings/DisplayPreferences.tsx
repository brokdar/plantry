import { Laptop, Moon, Sun } from "lucide-react"
import { useTranslation } from "react-i18next"

import { SettingsCard } from "@/components/editorial/SettingsCard"
import { useTheme } from "@/components/theme-provider"
import { cn } from "@/lib/utils"

const OPTIONS = [
  { value: "light", labelKey: "display_preferences.theme_light", Icon: Sun },
  { value: "dark", labelKey: "display_preferences.theme_dark", Icon: Moon },
  {
    value: "system",
    labelKey: "display_preferences.theme_system",
    Icon: Laptop,
  },
] as const

export function DisplayPreferences() {
  const { t } = useTranslation()
  const { theme, setTheme } = useTheme()

  return (
    <SettingsCard title={t("display_preferences.title")}>
      <div className="space-y-4">
        <div>
          <h4 className="text-sm font-semibold text-on-surface">
            {t("display_preferences.theme_label")}
          </h4>
          <p className="text-xs text-on-surface-variant">
            {t("display_preferences.theme_description")}
          </p>
        </div>
        <div
          role="radiogroup"
          aria-label={t("display_preferences.theme_label")}
          className="inline-flex rounded-full bg-surface-container-low p-1"
        >
          {OPTIONS.map(({ value, labelKey, Icon }) => {
            const active = theme === value
            return (
              <button
                key={value}
                type="button"
                role="radio"
                aria-checked={active}
                onClick={() => setTheme(value)}
                data-testid={`theme-option-${value}`}
                className={cn(
                  "flex items-center gap-2 rounded-full px-4 py-2 text-xs font-semibold transition-all",
                  active
                    ? "editorial-shadow bg-surface-container-lowest text-primary"
                    : "text-on-surface-variant hover:text-on-surface"
                )}
              >
                <Icon className="size-3.5" aria-hidden />
                {t(labelKey)}
              </button>
            )
          })}
        </div>
      </div>
    </SettingsCard>
  )
}
