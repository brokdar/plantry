import { zodResolver } from "@hookform/resolvers/zod"
import { Plus, X } from "lucide-react"
import { useRef, useState } from "react"
import { useForm, useWatch, type Resolver } from "react-hook-form"
import { useTranslation } from "react-i18next"
import { z } from "zod"

import { Button } from "@/components/ui/button"
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { DIET_TYPES, type DietType } from "@/lib/api/profile"
import {
  NUTRITION_PRESETS,
  NUTRITION_PRESET_IDS,
  macroSumValid,
  matchNutritionPreset,
  type NutritionPresetId,
  type NutritionTargets,
} from "@/lib/nutrition-presets"

import { OptionCards } from "./controls"

/*
 * Controlled profile editors shared by Settings → You & household and the
 * setup wizard. They hold no server state: the owner decides when to save.
 */

/** Diet type as four cards. */
export function DietTypePicker({
  value,
  onChange,
}: {
  value: DietType
  onChange: (next: DietType) => void
}) {
  const { t } = useTranslation()
  return (
    <OptionCards
      name="diet-type"
      legend={t("settings_page.household.diet_label")}
      value={value}
      onChange={onChange}
      options={DIET_TYPES.map((d) => ({
        value: d,
        label: t(`settings_page.household.diet.${d}`),
      }))}
    />
  )
}

/** Allergies and restrictions as removable chips (`dietary_restrictions`). */
export function AllergiesInput({
  value,
  onChange,
}: {
  value: string[]
  onChange: (next: string[]) => void
}) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState("")
  const inputRef = useRef<HTMLInputElement>(null)

  function add() {
    const trimmed = draft.trim()
    if (!trimmed || value.includes(trimmed)) return
    onChange([...value, trimmed])
    setDraft("")
    inputRef.current?.focus()
  }

  return (
    <div className="flex flex-col gap-2">
      {value.length > 0 && (
        <ul className="flex flex-wrap gap-2">
          {value.map((tag) => (
            <li
              key={tag}
              className="flex items-center gap-1 rounded-cell border border-line bg-panel py-0.5 pr-0.5 pl-2 text-body text-ink"
            >
              {tag}
              <button
                type="button"
                onClick={() => onChange(value.filter((r) => r !== tag))}
                className="flex size-6 items-center justify-center rounded-cell text-ink-muted hover:text-danger pointer-coarse:size-11"
                aria-label={t("profile.remove_restriction", { tag })}
              >
                <X className="size-4" strokeWidth={1.5} aria-hidden />
              </button>
            </li>
          ))}
        </ul>
      )}
      <div className="flex gap-2">
        <Input
          ref={inputRef}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder={t("profile.restriction_placeholder")}
          aria-label={t("settings_page.household.allergies_label")}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault()
              add()
            }
          }}
          className="max-w-xs pointer-coarse:h-11"
        />
        <Button
          type="button"
          variant="outline"
          onClick={add}
          className="pointer-coarse:h-11"
        >
          <Plus data-icon="inline-start" strokeWidth={1.5} aria-hidden />
          {t("profile.add_restriction")}
        </Button>
      </div>
    </div>
  )
}

const goalSchema = z.object({
  kcal_target: z.number().positive().nullable(),
  protein_pct: z.number().min(0).nullable(),
  fat_pct: z.number().min(0).nullable(),
  carbs_pct: z.number().min(0).nullable(),
})

type GoalValues = z.infer<typeof goalSchema>

type GoalChoice = NutritionPresetId | "custom"

/**
 * Nutrition goal: the D6 preset cards plus custom calorie and macro targets.
 * Picking a preset commits its numbers at once; custom fields commit on blur
 * once they are valid (macros add up to at most 100 %).
 */
export function NutritionGoalFields({
  value,
  onCommit,
}: {
  value: NutritionTargets
  onCommit: (next: NutritionTargets) => void
}) {
  const { t } = useTranslation()
  const matched = matchNutritionPreset(value)
  const [customOpen, setCustomOpen] = useState(matched === "custom")
  const choice: GoalChoice | null = customOpen ? "custom" : matched

  const form = useForm<GoalValues>({
    resolver: zodResolver(goalSchema) as Resolver<GoalValues>,
    values: value,
    resetOptions: { keepDirtyValues: true },
  })

  function pick(next: GoalChoice) {
    if (next === "custom") {
      setCustomOpen(true)
      return
    }
    setCustomOpen(false)
    form.reset(NUTRITION_PRESETS[next])
    onCommit(NUTRITION_PRESETS[next])
  }

  const commitCustom = form.handleSubmit((values) => {
    if (!macroSumValid(values)) {
      form.setError("root", { type: "macro_sum" })
      return
    }
    const unchanged =
      values.kcal_target === value.kcal_target &&
      values.protein_pct === value.protein_pct &&
      values.fat_pct === value.fat_pct &&
      values.carbs_pct === value.carbs_pct
    form.reset(values)
    if (!unchanged) onCommit(values)
  })

  const unit = t("units.kcal")
  const options = [
    ...NUTRITION_PRESET_IDS.map((id) => {
      const p = NUTRITION_PRESETS[id]
      return {
        value: id as GoalChoice,
        label: t(`settings_page.household.goal.${id}`),
        detail: (
          <>
            <span>
              {p.kcal_target} {unit}
            </span>
            <span>
              {t("settings_page.household.goal_split", {
                protein: p.protein_pct,
                fat: p.fat_pct,
                carbs: p.carbs_pct,
              })}
            </span>
          </>
        ),
      }
    }),
    {
      value: "custom" as GoalChoice,
      label: t("settings_page.household.goal.custom"),
      detail: t("settings_page.household.goal_custom_detail"),
    },
  ]

  const rootError = form.formState.errors.root
  const macros = useWatch({
    control: form.control,
    name: ["protein_pct", "fat_pct", "carbs_pct"],
  })
  const macroSum = macros.reduce<number>((sum, v) => sum + (v ?? 0), 0)

  return (
    <div className="flex flex-col gap-4">
      <OptionCards
        name="nutrition-goal"
        legend={t("settings_page.household.goal_label")}
        value={choice}
        onChange={pick}
        options={options}
      />
      {choice === "custom" && (
        <Form {...form}>
          <form
            noValidate
            onSubmit={(e) => {
              e.preventDefault()
              void commitCustom()
            }}
            // Commit when focus leaves the group, so moving between the
            // macro fields doesn't save a half-edited split.
            onBlur={(e) => {
              if (!e.currentTarget.contains(e.relatedTarget as Node | null)) {
                void commitCustom()
              }
            }}
            className="flex flex-col gap-3"
          >
            <FormField
              control={form.control}
              name="kcal_target"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("profile.kcal_target")}</FormLabel>
                  <FormControl>
                    <Input
                      type="number"
                      inputMode="numeric"
                      min={1}
                      placeholder={t("profile.kcal_target_placeholder")}
                      className="w-32 tabular-nums pointer-coarse:h-11"
                      value={field.value ?? ""}
                      onChange={(e) =>
                        field.onChange(
                          e.target.value === "" ? null : Number(e.target.value)
                        )
                      }
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className="grid grid-cols-3 items-start gap-3">
              {(
                [
                  ["protein_pct", "profile.protein_pct"],
                  ["fat_pct", "profile.fat_pct"],
                  ["carbs_pct", "profile.carbs_pct"],
                ] as const
              ).map(([name, labelKey]) => (
                <FormField
                  key={name}
                  control={form.control}
                  name={name}
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t(labelKey)}</FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          inputMode="numeric"
                          min={0}
                          max={100}
                          step={1}
                          placeholder="0"
                          className="tabular-nums pointer-coarse:h-11"
                          value={field.value ?? ""}
                          onChange={(e) =>
                            field.onChange(
                              e.target.value === ""
                                ? null
                                : Number(e.target.value)
                            )
                          }
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              ))}
            </div>
            {rootError ? (
              <p role="alert" className="text-small text-danger">
                {t("settings_page.household.macro_sum_invalid")}
              </p>
            ) : (
              <p className="text-small text-ink-muted tabular-nums">
                {t("profile.macro_sum", { sum: macroSum })}
              </p>
            )}
          </form>
        </Form>
      )}
    </div>
  )
}
