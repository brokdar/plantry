import { useTranslation } from "react-i18next"

import { gridValueToDefaults } from "@/lib/availability-grid"
import { useSetAvailabilityDefaults } from "@/lib/queries/availability"
import { usePatchProfile } from "@/lib/queries/profile"
import { useSetSetting, useSettingItems } from "@/lib/queries/settings"
import { useCreateTimeSlot, useUpdateTimeSlot } from "@/lib/queries/slots"
import { NUTRITION_PRESETS } from "@/lib/nutrition-presets"

import type { AvailabilityGridValue } from "@/components/availability/AvailabilityGrid"

import {
  COOKING_DEFAULTS,
  SETTING_KEYS,
  VARIETY_DEFAULTS,
  slotNameKey,
  type AssistantDraft,
  type CookingDraft,
  type MealsDraft,
  type VarietyDraft,
  type WizardDraft,
} from "./wizard-draft"

/** Step indices, 0-based. */
export const STEP_COUNT = 7

/**
 * Persists one step of the wizard with the existing mutations. Every saver
 * rejects when a request fails; the mutation hooks already toast the error,
 * so the wizard only has to stay on the step.
 */
export function useWizardSave() {
  const { t, i18n } = useTranslation()
  const patchProfile = usePatchProfile()
  const setSetting = useSetSetting()
  const createSlot = useCreateTimeSlot()
  const updateSlot = useUpdateTimeSlot()
  const setDefaults = useSetAvailabilityDefaults()
  const { data: stored } = useSettingItems()

  // Only values that differ from the stored ones are written, so accepting
  // a default doesn't turn it into a database override.
  const settings = (pairs: [string, string][]) =>
    Promise.all(
      pairs
        .filter(([key, value]) => stored?.[key]?.value !== value)
        .map(([key, value]) => setSetting.mutateAsync({ key, value }))
    )

  /** Creates or updates the slots, then writes the defaults grid over the
   *  active ones. Returns the meals draft with the real slot ids. */
  async function saveMeals(meals: MealsDraft): Promise<MealsDraft> {
    const saved = await Promise.all(
      meals.slots.map((slot, sort_order) => {
        const input = {
          name_key: slotNameKey(t, slot),
          icon: slot.icon,
          sort_order,
          active: slot.active,
        }
        return slot.id < 0
          ? createSlot.mutateAsync(input)
          : updateSlot.mutateAsync({ id: slot.id, data: input })
      })
    )
    const idMap = new Map(meals.slots.map((s, i) => [s.id, saved[i]!.id]))
    const grid: AvailabilityGridValue = {}
    for (const [key, cell] of Object.entries(meals.grid)) {
      const [weekday, slotId] = key.split(":")
      const id = idMap.get(Number(slotId))
      if (id !== undefined) grid[`${weekday}:${id}`] = cell
    }
    const active = meals.slots
      .map((s, i) => ({ ...s, id: saved[i]!.id }))
      .filter((s) => s.active)
    await setDefaults.mutateAsync(gridValueToDefaults(grid, active))
    return {
      slots: meals.slots.map((s, i) => ({
        ...s,
        id: saved[i]!.id,
        name_key: saved[i]!.name_key,
      })),
      grid,
    }
  }

  const saveCooking = (c: CookingDraft) =>
    settings([
      [SETTING_KEYS.occasions, String(c.occasions)],
      [SETTING_KEYS.batch, String(c.batch)],
      [SETTING_KEYS.keepDays, String(c.keepDays)],
      [SETTING_KEYS.freezing, String(c.freezing)],
    ])

  const saveVariety = (v: VarietyDraft) =>
    settings([
      [SETTING_KEYS.varietyWeeks, String(v.varietyWeeks)],
      [SETTING_KEYS.shoppingDay, String(v.shoppingDay)],
      [SETTING_KEYS.explorePerWeek, String(v.explorePerWeek)],
    ])

  async function saveAssistant(a: AssistantDraft) {
    await Promise.all([
      patchProfile.mutateAsync({
        system_prompt: a.notes.trim() === "" ? null : a.notes,
      }),
      settings([[SETTING_KEYS.daily, String(a.daily)]]),
    ])
  }

  /** Saves step `step` and returns the draft as stored. */
  async function saveStep(
    step: number,
    draft: WizardDraft
  ): Promise<WizardDraft> {
    switch (step) {
      case 0:
        await patchProfile.mutateAsync({ locale: draft.language })
        await i18n.changeLanguage(draft.language)
        return draft
      case 1:
        await patchProfile.mutateAsync(draft.household)
        return draft
      case 2:
        await patchProfile.mutateAsync(draft.nutrition)
        return draft
      case 3:
        return { ...draft, meals: await saveMeals(draft.meals) }
      case 4:
        await saveCooking(draft.cooking)
        return draft
      case 5:
        await saveVariety(draft.variety)
        return draft
      default:
        await saveAssistant(draft.assistant)
        return draft
    }
  }

  /**
   * Plan D16: saves the current step as shown, then writes the defaults of
   * every later step. Step 4 keeps the slots it shows (the defaults on an
   * install with none) with every cell planned.
   */
  async function skipFrom(step: number, draft: WizardDraft) {
    const stored = await saveStep(step, draft)
    const later: Promise<unknown>[] = []
    if (step < 2)
      later.push(patchProfile.mutateAsync(NUTRITION_PRESETS.maintain))
    if (step < 3) later.push(saveMeals({ slots: stored.meals.slots, grid: {} }))
    if (step < 4) later.push(saveCooking(COOKING_DEFAULTS))
    if (step < 5) later.push(saveVariety(VARIETY_DEFAULTS))
    if (step < 6) later.push(settings([[SETTING_KEYS.daily, "true"]]))
    await Promise.all(later)
  }

  return { saveStep, skipFrom }
}
