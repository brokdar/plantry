import type { TFunction } from "i18next"

import type { AvailabilityGridValue } from "@/components/availability/AvailabilityGrid"
import { defaultsToGridValue } from "@/lib/availability-grid"
import type { AvailabilityDefault } from "@/lib/api/availability"
import type { DietType, Profile } from "@/lib/api/profile"
import type { SettingItem } from "@/lib/api/settings"
import type { TimeSlot } from "@/lib/api/slots"
import {
  NUTRITION_PRESETS,
  type NutritionTargets,
} from "@/lib/nutrition-presets"
import { slotLabel } from "@/lib/slot-label"

/*
 * The wizard's local copy of every step's values. Each step edits its slice;
 * Next saves the slice. Built once from the stored values, so a re-run opens
 * prefilled.
 */

export type Language = "en" | "de"

export interface HouseholdDraft {
  diet_type: DietType
  household_portions: number
  dietary_restrictions: string[]
  meat_days_per_week_max: number | null
}

/** A slot as shown in step 4. `id` is negative until the slot is created. */
export interface SlotDraft {
  id: number
  name_key: string
  /** The name as typed; becomes `name_key` when it no longer matches the
   *  translated label of the original key. */
  label: string
  icon: string
  active: boolean
}

export interface MealsDraft {
  slots: SlotDraft[]
  grid: AvailabilityGridValue
}

export interface CookingDraft {
  occasions: number
  batch: boolean
  keepDays: number
  freezing: boolean
}

export interface VarietyDraft {
  varietyWeeks: number
  shoppingDay: number
  explorePerWeek: number
}

export interface AssistantDraft {
  notes: string
  daily: boolean
}

export interface WizardDraft {
  language: Language
  household: HouseholdDraft
  nutrition: NutritionTargets
  meals: MealsDraft
  cooking: CookingDraft
  variety: VarietyDraft
  assistant: AssistantDraft
}

/** Saturday: the wizard's suggested shopping day (0 = Monday). */
export const WIZARD_SHOPPING_DAY = 5

export const COOKING_DEFAULTS: CookingDraft = {
  occasions: 1,
  batch: true,
  keepDays: 3,
  freezing: false,
}

export const VARIETY_DEFAULTS: VarietyDraft = {
  varietyWeeks: 3,
  shoppingDay: WIZARD_SHOPPING_DAY,
  explorePerWeek: 0,
}

/** Plan D15: the slots an install with none starts with. */
export const DEFAULT_SLOTS: readonly { name_key: string; icon: string }[] = [
  { name_key: "slot.breakfast", icon: "Coffee" },
  { name_key: "slot.lunch", icon: "Sun" },
  { name_key: "slot.dinner", icon: "Moon" },
]

/** The settings keys steps 5–7 write. */
export const SETTING_KEYS = {
  occasions: "plan.max_cook_occasions_per_day",
  batch: "plan.batch_cooking",
  keepDays: "plan.default_keep_days",
  freezing: "plan.freezing_enabled",
  varietyWeeks: "plan.variety_weeks",
  shoppingDay: "plan.shopping_day",
  explorePerWeek: "plan.explore_per_week",
  daily: "discovery.daily_enabled",
} as const

export function defaultSlotDrafts(t: TFunction): SlotDraft[] {
  return DEFAULT_SLOTS.map((s, i) => ({
    id: -(i + 1),
    name_key: s.name_key,
    label: slotLabel(t, s.name_key),
    icon: s.icon,
    active: true,
  }))
}

/** The browser's language when it is one Plantry speaks, else English. */
export function toLanguage(lang: string | undefined): Language {
  return lang?.split("-")[0] === "de" ? "de" : "en"
}

export function buildDraft({
  t,
  language,
  profile,
  settings,
  slots,
  defaults,
}: {
  t: TFunction
  language: string
  profile: Profile
  settings: Record<string, SettingItem>
  slots: TimeSlot[]
  defaults: AvailabilityDefault[]
}): WizardDraft {
  const num = (key: string, fallback: number) => {
    const v = settings[key]?.value
    return v ? Number(v) : fallback
  }
  const bool = (key: string, fallback: boolean) => {
    const v = settings[key]?.value
    return v ? v === "true" : fallback
  }
  const shopping = settings[SETTING_KEYS.shoppingDay]
  const active = slots
    .filter((s) => s.active)
    .sort((a, b) => a.sort_order - b.sort_order)

  return {
    language: toLanguage(language),
    household: {
      diet_type: profile.diet_type,
      household_portions: profile.household_portions,
      dietary_restrictions: profile.dietary_restrictions ?? [],
      meat_days_per_week_max: profile.meat_days_per_week_max,
    },
    nutrition:
      profile.kcal_target == null
        ? NUTRITION_PRESETS.maintain
        : {
            kcal_target: profile.kcal_target,
            protein_pct: profile.protein_pct,
            fat_pct: profile.fat_pct,
            carbs_pct: profile.carbs_pct,
          },
    meals: {
      slots:
        active.length > 0
          ? active.map((s) => ({
              id: s.id,
              name_key: s.name_key,
              label: slotLabel(t, s.name_key),
              icon: s.icon,
              active: true,
            }))
          : defaultSlotDrafts(t),
      grid: defaultsToGridValue(defaults),
    },
    cooking: {
      occasions: num(SETTING_KEYS.occasions, COOKING_DEFAULTS.occasions),
      batch: bool(SETTING_KEYS.batch, COOKING_DEFAULTS.batch),
      keepDays: num(SETTING_KEYS.keepDays, COOKING_DEFAULTS.keepDays),
      freezing: bool(SETTING_KEYS.freezing, COOKING_DEFAULTS.freezing),
    },
    variety: {
      varietyWeeks: num(
        SETTING_KEYS.varietyWeeks,
        VARIETY_DEFAULTS.varietyWeeks
      ),
      // The registry default is Monday; the wizard suggests Saturday until
      // the user has picked a day.
      shoppingDay:
        shopping?.source === "db" || shopping?.source === "env"
          ? Number(shopping.value)
          : WIZARD_SHOPPING_DAY,
      explorePerWeek: num(
        SETTING_KEYS.explorePerWeek,
        VARIETY_DEFAULTS.explorePerWeek
      ),
    },
    assistant: {
      notes: profile.system_prompt ?? "",
      daily: bool(SETTING_KEYS.daily, true),
    },
  }
}

/** The `name_key` to store for a slot: the original key while the name
 *  still matches its translation, else the typed name. */
export function slotNameKey(t: TFunction, slot: SlotDraft): string {
  const label = slot.label.trim()
  return label === slotLabel(t, slot.name_key) ? slot.name_key : label
}

/** Why Next is blocked, or null: every slot needs a name and one must be on. */
export function mealsError(
  value: MealsDraft
): "unnamed" | "none_active" | null {
  if (value.slots.some((s) => s.label.trim() === "")) return "unnamed"
  if (!value.slots.some((s) => s.active)) return "none_active"
  return null
}
