import { useTranslation } from "react-i18next"

import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import type { Profile, ProfileInput } from "@/lib/api/profile"
import type { NutritionTargets } from "@/lib/nutrition-presets"
import { usePatchProfile, useProfile } from "@/lib/queries/profile"

import { SettingsGroup } from "../controls"
import { useSavedNote } from "../setting-field"
import { formatPortions } from "../household"
import { HouseholdHelper } from "../HouseholdHelper"
import {
  AllergiesInput,
  DietTypePicker,
  NutritionGoalFields,
} from "../ProfileEditor"
import { SettingRow } from "../SettingRow"

const NO_LIMIT = "none"
const MEAT_DAY_OPTIONS = [0, 1, 2, 3, 4, 5, 6, 7]

/** Saves a profile patch and flashes this row's "Saved" note. */
function useProfileRow() {
  const patch = usePatchProfile()
  const { saved, markSaved } = useSavedNote()
  return {
    saved,
    save: (input: ProfileInput) =>
      patch.mutate(input, { onSuccess: markSaved }),
  }
}

export function HouseholdSection() {
  const { data: profile } = useProfile()
  if (!profile) return <SectionSkeleton />
  return <HouseholdFields profile={profile} />
}

function HouseholdFields({ profile }: { profile: Profile }) {
  const { t, i18n } = useTranslation()
  const diet = useProfileRow()
  const household = useProfileRow()
  const allergies = useProfileRow()
  const meat = useProfileRow()
  const goal = useProfileRow()

  const restrictions = profile.dietary_restrictions ?? []
  const meatDays = profile.meat_days_per_week_max
  const targets: NutritionTargets = {
    kcal_target: profile.kcal_target,
    protein_pct: profile.protein_pct,
    fat_pct: profile.fat_pct,
    carbs_pct: profile.carbs_pct,
  }

  return (
    <SettingsGroup>
      <SettingRow
        label={t("settings_page.household.diet_label")}
        description={t(
          `settings_page.household.diet_effect.${profile.diet_type}`
        )}
        saved={diet.saved}
      >
        <DietTypePicker
          value={profile.diet_type}
          onChange={(diet_type) => diet.save({ diet_type })}
        />
      </SettingRow>

      <SettingRow
        label={t("settings_page.household.household_label")}
        description={
          <>
            {t("settings_page.household.household_effect", {
              count: profile.household_portions,
              portions: formatPortions(
                profile.household_portions,
                i18n.language
              ),
            })}{" "}
            {t("settings_page.household.household_hint")}
          </>
        }
        saved={household.saved}
      >
        <HouseholdHelper
          value={profile.household_portions}
          onChange={(household_portions) =>
            household.save({ household_portions })
          }
        />
      </SettingRow>

      <SettingRow
        label={t("settings_page.household.allergies_label")}
        description={
          restrictions.length > 0
            ? t("settings_page.household.allergies_effect")
            : t("settings_page.household.allergies_effect_none")
        }
        saved={allergies.saved}
      >
        <AllergiesInput
          value={restrictions}
          onChange={(dietary_restrictions) =>
            allergies.save({ dietary_restrictions })
          }
        />
      </SettingRow>

      {profile.diet_type === "omnivore" && (
        <SettingRow
          label={t("settings_page.household.meat_days_label")}
          description={meatDaysEffect(t, meatDays)}
          saved={meat.saved}
          htmlFor="settings-meat-days"
        >
          <Select
            value={meatDays == null ? NO_LIMIT : String(meatDays)}
            onValueChange={(v) =>
              meat.save({
                meat_days_per_week_max: v === NO_LIMIT ? null : Number(v),
              })
            }
          >
            <SelectTrigger
              id="settings-meat-days"
              className="w-full max-w-48 pointer-coarse:h-11"
              data-testid="meat-days-select"
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
        </SettingRow>
      )}

      <SettingRow
        label={t("settings_page.household.goal_label")}
        description={goalEffect(t, targets)}
        saved={goal.saved}
      >
        <NutritionGoalFields
          value={targets}
          onCommit={(next) => goal.save(next)}
        />
      </SettingRow>
    </SettingsGroup>
  )
}

type T = ReturnType<typeof useTranslation>["t"]

function meatDaysEffect(t: T, days: number | null): string {
  if (days == null) return t("settings_page.household.meat_days_effect_none")
  if (days === 0) return t("settings_page.household.meat_days_effect_zero")
  return t("settings_page.household.meat_days_effect", { count: days })
}

function goalEffect(t: T, g: NutritionTargets): string {
  if (g.kcal_target == null) {
    return t("settings_page.household.goal_effect_none")
  }
  if (g.protein_pct == null || g.fat_pct == null || g.carbs_pct == null) {
    return t("settings_page.household.goal_effect_kcal_only", {
      kcal: g.kcal_target,
    })
  }
  return t("settings_page.household.goal_effect", {
    kcal: g.kcal_target,
    protein: g.protein_pct,
    fat: g.fat_pct,
    carbs: g.carbs_pct,
  })
}

export function SectionSkeleton() {
  return (
    <div className="flex flex-col gap-4" aria-hidden>
      <Skeleton className="h-16 w-full" />
      <Skeleton className="h-16 w-full" />
      <Skeleton className="h-16 w-3/4" />
    </div>
  )
}
