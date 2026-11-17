import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  ACTIVITY_LEVELS,
  GOALS,
  type ActivityLevel,
  type Goal,
  type Profile,
  type ProfileInput,
  type Sex,
} from "@/lib/api/profile"
import { estimateKcal } from "@/lib/nutrition-estimate"

import { OptionCards } from "./controls"

/** The nutrition goal as three cards. */
export function GoalPicker({
  value,
  onChange,
}: {
  value: Goal
  onChange: (next: Goal) => void
}) {
  const { t } = useTranslation()
  return (
    <OptionCards
      name="nutrition-goal-mode"
      legend={t("settings_page.household.policy.goal_label")}
      value={value}
      onChange={onChange}
      columns={3}
      options={GOALS.map((g) => ({
        value: g,
        label: t(`settings_page.household.policy.goal.${g}`),
      }))}
    />
  )
}

/** A whole or decimal number that saves on blur; empty clears it. */
function BodyNumber({
  label,
  value,
  unit,
  min,
  max,
  step,
  onCommit,
}: {
  label: string
  value: number | null
  unit?: string
  min: number
  max: number
  step: number
  onCommit: (next: number | null) => void
}) {
  const [draft, setDraft] = useState<string | null>(null)
  const text = draft ?? (value == null ? "" : String(value))

  function commit() {
    if (draft == null) return
    setDraft(null)
    if (draft.trim() === "") {
      if (value != null) onCommit(null)
      return
    }
    const n = Number(draft)
    if (Number.isFinite(n) && n >= min && n <= max && n !== value) onCommit(n)
  }

  return (
    <label className="flex flex-col gap-1">
      <span className="text-small text-ink-muted">{label}</span>
      <span className="flex items-center gap-2">
        <Input
          type="number"
          inputMode="decimal"
          min={min}
          max={max}
          step={step}
          value={text}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === "Enter") e.currentTarget.blur()
          }}
          className="w-28 tabular-nums pointer-coarse:h-11"
        />
        {unit && <span className="text-small text-ink-muted">{unit}</span>}
      </span>
    </label>
  )
}

const UNSET = "unset"

/**
 * Optional body data and the "suggest my target" helper. The estimate
 * (Mifflin-St Jeor x activity level) is shown before it is applied, labelled
 * as an estimate; applying writes the editable kcal target.
 */
export function BodyFields({
  profile,
  onSave,
}: {
  profile: Profile
  onSave: (input: ProfileInput) => void
}) {
  const { t, i18n } = useTranslation()
  const estimate = estimateKcal(profile, profile.goal)
  const nf = new Intl.NumberFormat(i18n.language)

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap gap-x-6 gap-y-3">
        <BodyNumber
          label={t("settings_page.household.policy.weight")}
          unit={t("units.kg")}
          value={profile.weight_kg}
          min={20}
          max={500}
          step={0.1}
          onCommit={(weight_kg) => onSave({ weight_kg })}
        />
        <BodyNumber
          label={t("settings_page.household.policy.height")}
          unit={t("units.cm")}
          value={profile.height_cm}
          min={50}
          max={280}
          step={1}
          onCommit={(height_cm) => onSave({ height_cm })}
        />
        <BodyNumber
          label={t("settings_page.household.policy.birth_year")}
          value={profile.birth_year}
          min={1900}
          max={new Date().getFullYear()}
          step={1}
          onCommit={(birth_year) => onSave({ birth_year })}
        />
      </div>
      <OptionCards
        name="body-sex"
        legend={t("settings_page.household.policy.sex_label")}
        value={(profile.sex ?? UNSET) as Sex | typeof UNSET}
        onChange={(v) => onSave({ sex: v === UNSET ? null : v })}
        columns={3}
        options={[
          {
            value: "female" as const,
            label: t("settings_page.household.policy.sex.female"),
          },
          {
            value: "male" as const,
            label: t("settings_page.household.policy.sex.male"),
          },
          {
            value: UNSET,
            label: t("settings_page.household.policy.sex.unset"),
          },
        ]}
      />
      <OptionCards
        name="body-activity"
        legend={t("settings_page.household.policy.activity_label")}
        value={
          profile.activity_level == null ? null : String(profile.activity_level)
        }
        onChange={(v) => onSave({ activity_level: Number(v) as ActivityLevel })}
        columns={3}
        options={ACTIVITY_LEVELS.map((a) => ({
          value: String(a),
          label: t(
            `settings_page.household.policy.activity.${String(a).replace(".", "_")}`
          ),
          detail: <span>{`× ${a}`}</span>,
        }))}
      />
      <div className="flex flex-col gap-2" data-testid="kcal-suggestion">
        {estimate ? (
          <>
            <p className="text-body text-ink">
              {t("settings_page.household.policy.estimate", {
                target: nf.format(estimate.target),
                maintenance: nf.format(estimate.maintenance),
              })}
            </p>
            <p className="text-small text-ink-muted">
              {t("settings_page.household.policy.estimate_note")}
            </p>
            <Button
              type="button"
              variant="outline"
              className="w-fit pointer-coarse:h-11"
              onClick={() => onSave({ kcal_target: estimate.target })}
            >
              {t("settings_page.household.policy.suggest")}
            </Button>
          </>
        ) : (
          <p className="text-small text-ink-muted">
            {t("settings_page.household.policy.estimate_missing")}
          </p>
        )}
      </div>
    </div>
  )
}
