import { useTranslation } from "react-i18next"

import { Stepper } from "@/components/settings/controls"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

import { SetupField } from "./SetupField"
import type { VarietyDraft } from "./wizard-draft"

const WEEKDAYS = [0, 1, 2, 3, 4, 5, 6]

export function VarietyStep({
  value,
  onChange,
}: {
  value: VarietyDraft
  onChange: (next: VarietyDraft) => void
}) {
  const { t } = useTranslation()
  const set = (patch: Partial<VarietyDraft>) => onChange({ ...value, ...patch })
  const varietyLabel = t("settings_page.cooking.variety_label")
  const exploreLabel = t("settings_page.cooking.explore_label")
  const dayName = (d: number) => t(`availability.weekday_long.${d}`)

  return (
    <div className="flex flex-col">
      <SetupField
        label={varietyLabel}
        description={t("settings_page.cooking.variety_effect", {
          count: value.varietyWeeks,
        })}
      >
        <Stepper
          label={varietyLabel}
          value={value.varietyWeeks}
          min={0}
          max={8}
          format={(count) => t("settings_page.weeks", { count })}
          onChange={(varietyWeeks) => set({ varietyWeeks })}
        />
      </SetupField>

      <SetupField
        label={t("settings.shopping_day")}
        description={t("setup.variety.shopping_why", {
          day: dayName(value.shoppingDay),
        })}
        htmlFor="setup-shopping-day"
      >
        <Select
          value={String(value.shoppingDay)}
          onValueChange={(v) => set({ shoppingDay: Number(v) })}
        >
          <SelectTrigger
            id="setup-shopping-day"
            className="w-full max-w-60 pointer-coarse:h-11"
            data-testid="setup-shopping-day"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {WEEKDAYS.map((d) => (
                <SelectItem key={d} value={String(d)}>
                  {dayName(d)}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </SetupField>

      <SetupField
        label={exploreLabel}
        description={t("settings_page.cooking.explore_effect", {
          count: value.explorePerWeek,
        })}
      >
        <Stepper
          label={exploreLabel}
          value={value.explorePerWeek}
          min={0}
          max={3}
          onChange={(explorePerWeek) => set({ explorePerWeek })}
        />
      </SetupField>
    </div>
  )
}
