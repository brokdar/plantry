import { useTranslation } from "react-i18next"

import { formatPortions } from "@/components/settings/household"
import { HouseholdHelper } from "@/components/settings/HouseholdHelper"
import {
  AllergiesInput,
  DietTypePicker,
} from "@/components/settings/ProfileEditor"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

import { SetupField } from "./SetupField"
import type { HouseholdDraft } from "./wizard-draft"

const NO_LIMIT = "none"
const MEAT_DAY_OPTIONS = [0, 1, 2, 3, 4, 5, 6, 7]

export function HouseholdStep({
  value,
  onChange,
}: {
  value: HouseholdDraft
  onChange: (next: HouseholdDraft) => void
}) {
  const { t, i18n } = useTranslation()
  const set = (patch: Partial<HouseholdDraft>) =>
    onChange({ ...value, ...patch })

  return (
    <div className="flex flex-col">
      <SetupField
        label={t("settings_page.household.diet_label")}
        description={t(
          `settings_page.household.diet_effect.${value.diet_type}`
        )}
      >
        <DietTypePicker
          value={value.diet_type}
          onChange={(diet_type) =>
            set({
              diet_type,
              // The limit only means something for omnivores.
              meat_days_per_week_max:
                diet_type === "omnivore" ? value.meat_days_per_week_max : null,
            })
          }
        />
      </SetupField>

      <SetupField
        label={t("settings_page.household.household_label")}
        description={t("settings_page.household.household_hint")}
      >
        <HouseholdHelper
          value={value.household_portions}
          onChange={(household_portions) => set({ household_portions })}
        />
        <p
          className="text-body text-ink tabular-nums"
          data-testid="setup-household-portions"
        >
          {t("setup.household.portions", {
            count: value.household_portions,
            portions: formatPortions(value.household_portions, i18n.language),
          })}
        </p>
      </SetupField>

      <SetupField
        label={t("settings_page.household.allergies_label")}
        description={t("setup.household.allergies_why")}
      >
        <AllergiesInput
          value={value.dietary_restrictions}
          onChange={(dietary_restrictions) => set({ dietary_restrictions })}
        />
      </SetupField>

      {value.diet_type === "omnivore" && (
        <SetupField
          label={t("settings_page.household.meat_days_label")}
          description={t("setup.household.meat_days_optional")}
          htmlFor="setup-meat-days"
        >
          <Select
            value={
              value.meat_days_per_week_max == null
                ? NO_LIMIT
                : String(value.meat_days_per_week_max)
            }
            onValueChange={(v) =>
              set({ meat_days_per_week_max: v === NO_LIMIT ? null : Number(v) })
            }
          >
            <SelectTrigger
              id="setup-meat-days"
              className="w-full max-w-48 pointer-coarse:h-11"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value={NO_LIMIT}>
                  {t("settings_page.household.meat_days_none")}
                </SelectItem>
                {MEAT_DAY_OPTIONS.map((n) => (
                  <SelectItem key={n} value={String(n)}>
                    {n}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </SetupField>
      )}
    </div>
  )
}
