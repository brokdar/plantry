import { Link } from "@tanstack/react-router"
import { ChevronRight, X } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { usePatchProfile, useProfile } from "@/lib/queries/profile"
import { useSettingItems } from "@/lib/queries/settings"

import { AIProviderControls } from "../AIProviderControls"
import { PricesEditor } from "../PricesEditor"
import { NumberField, SettingsGroup, Switch } from "../controls"
import { useSavedNote, useSettingField } from "../setting-field"
import { SettingRow } from "../SettingRow"
import { SectionSkeleton } from "./HouseholdSection"

export function AssistantSection() {
  const { t } = useTranslation()
  const { data } = useSettingItems()
  if (!data) return <SectionSkeleton />

  return (
    <div className="flex flex-col gap-10">
      <SettingsGroup title={t("settings_page.assistant.connection_group")}>
        <AIProviderControls />
      </SettingsGroup>

      <SettingsGroup>
        <NotesRow />
        <DailySuggestionRow />
        <CuisinesRow />
        <RetentionRow />
      </SettingsGroup>

      <SettingsGroup>
        <SettingRow
          label={t("settings_page.insights.title")}
          description={t("settings_page.assistant.insights_effect")}
        >
          <Button asChild variant="outline" className="self-start">
            <Link to="/settings/assistant/insights" data-testid="insights-link">
              {t("settings_page.assistant.insights_open")}
              <ChevronRight strokeWidth={1.5} aria-hidden />
            </Link>
          </Button>
        </SettingRow>
      </SettingsGroup>

      <PricesEditor />

      <SettingsGroup title={t("settings_page.ai.advanced_title")}>
        <RateLimitRow />
        <FakeScriptRow />
      </SettingsGroup>
    </div>
  )
}

/** `profile.system_prompt`, saved on blur. */
function NotesRow() {
  const { t } = useTranslation()
  const { data: profile } = useProfile()
  const patch = usePatchProfile()
  const { saved, markSaved } = useSavedNote()
  const [draft, setDraft] = useState<string | null>(null)
  const stored = profile?.system_prompt ?? ""
  const text = draft ?? stored

  function commit() {
    if (draft === null) return
    setDraft(null)
    if (draft === stored) return
    patch.mutate(
      { system_prompt: draft.trim() === "" ? null : draft },
      { onSuccess: markSaved }
    )
  }

  return (
    <SettingRow
      label={t("settings_page.assistant.notes_label")}
      description={t("settings_page.assistant.notes_effect")}
      saved={saved}
      htmlFor="assistant-notes"
    >
      <Textarea
        id="assistant-notes"
        rows={4}
        placeholder={t("settings_page.assistant.notes_placeholder")}
        value={text}
        disabled={!profile}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commit}
        data-testid="assistant-notes"
      />
    </SettingRow>
  )
}

function DailySuggestionRow() {
  const { t } = useTranslation()
  const field = useSettingField("discovery.daily_enabled", "true")
  const on = field.value === "true"
  return (
    <SettingRow
      label={t("settings_page.assistant.daily_label")}
      description={t(
        on
          ? "settings_page.assistant.daily_effect_on"
          : "settings_page.assistant.daily_effect_off"
      )}
      item={field.item}
      onClearOverride={field.clear}
      saved={field.saved}
      htmlFor="setting-daily-suggestion"
    >
      <Switch
        id="setting-daily-suggestion"
        checked={on}
        onChange={(next) => field.save(String(next))}
      />
    </SettingRow>
  )
}

/** At most this many cuisines (D19, the backend validates the same). */
const MAX_CUISINES = 10

function parseCuisines(value: string): string[] {
  try {
    const list: unknown = JSON.parse(value)
    return Array.isArray(list)
      ? list.filter((c): c is string => typeof c === "string")
      : []
  } catch {
    return []
  }
}

/** `discovery.cuisines` as chips (D19): lowercase names the daily idea
 *  rotates through, saved as a JSON array on every add and remove. */
function CuisinesRow() {
  const { t } = useTranslation()
  const field = useSettingField("discovery.cuisines", "[]")
  const cuisines = parseCuisines(field.value)
  const [draft, setDraft] = useState("")
  const full = cuisines.length >= MAX_CUISINES

  function add() {
    const name = draft.trim().toLowerCase()
    setDraft("")
    if (!name || full || cuisines.includes(name)) return
    field.save(JSON.stringify([...cuisines, name]))
  }

  return (
    <SettingRow
      label={t("settings_page.assistant.cuisines_label")}
      description={t("settings_page.assistant.cuisines_effect")}
      item={field.item}
      onClearOverride={field.clear}
      saved={field.saved}
      htmlFor="setting-cuisines"
    >
      <div className="flex flex-col gap-3">
        {cuisines.length > 0 && (
          <ul className="flex flex-wrap gap-2" data-testid="cuisine-chips">
            {cuisines.map((name) => (
              <li
                key={name}
                className="inline-flex items-center gap-1 rounded-full border border-line bg-panel py-1 pr-1 pl-3 text-small text-ink"
              >
                {name}
                <Button
                  type="button"
                  variant="ghost"
                  className="size-8 rounded-full md:pointer-fine:size-6"
                  aria-label={t("settings_page.assistant.cuisines_remove", {
                    name,
                  })}
                  onClick={() =>
                    field.save(
                      JSON.stringify(cuisines.filter((c) => c !== name))
                    )
                  }
                >
                  <X strokeWidth={1.5} aria-hidden />
                </Button>
              </li>
            ))}
          </ul>
        )}
        <div className="flex gap-2">
          <Input
            id="setting-cuisines"
            value={draft}
            maxLength={40}
            disabled={full}
            placeholder={t("settings_page.assistant.cuisines_placeholder")}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault()
                add()
              }
            }}
            className="max-w-xs pointer-coarse:h-11"
            data-testid="cuisine-input"
          />
          <Button
            type="button"
            variant="outline"
            disabled={full}
            onClick={add}
            className="pointer-coarse:h-11"
          >
            {t("settings_page.assistant.cuisines_add")}
          </Button>
        </div>
        {full && (
          <p className="text-small text-ink-muted">
            {t("settings_page.assistant.cuisines_full", {
              count: MAX_CUISINES,
            })}
          </p>
        )}
      </div>
    </SettingRow>
  )
}

function RetentionRow() {
  const { t } = useTranslation()
  const field = useSettingField("ai.run_retention_days", "180")
  const days = Number(field.value)
  return (
    <SettingRow
      label={t("settings_page.assistant.retention_label")}
      description={t("settings_page.assistant.retention_effect", {
        count: days,
      })}
      item={field.item}
      onClearOverride={field.clear}
      saved={field.saved}
      htmlFor="setting-run-retention"
    >
      <NumberField
        id="setting-run-retention"
        value={days}
        min={7}
        max={3650}
        suffix={t("settings_page.days_unit", { count: days })}
        onCommit={(n) => field.save(String(n))}
        testId="run-retention-input"
      />
    </SettingRow>
  )
}

function RateLimitRow() {
  const { t } = useTranslation()
  const field = useSettingField("ai.rate_limit_per_min", "10")
  const perMin = Number(field.value)
  return (
    <SettingRow
      label={t("settings_page.ai.rate_limit_label")}
      description={t("settings_page.assistant.rate_limit_effect", {
        count: perMin,
      })}
      item={field.item}
      onClearOverride={field.clear}
      saved={field.saved}
      htmlFor="setting-rate-limit"
    >
      <NumberField
        id="setting-rate-limit"
        value={perMin}
        min={0}
        max={10000}
        onCommit={(n) => field.save(String(n))}
        testId="ai-rate-limit-input"
      />
    </SettingRow>
  )
}

function FakeScriptRow() {
  const { t } = useTranslation()
  const field = useSettingField("ai.fake_script", "")
  const [draft, setDraft] = useState<string | null>(null)
  const text = draft ?? field.value

  function commit() {
    if (draft === null) return
    setDraft(null)
    if (draft !== field.value) field.save(draft)
  }

  return (
    <SettingRow
      label={t("settings_page.ai.fake_script_label")}
      description={t("settings_page.ai.fake_script_description")}
      item={field.item}
      onClearOverride={field.clear}
      saved={field.saved}
      htmlFor="setting-fake-script"
    >
      <Input
        id="setting-fake-script"
        value={text}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commit}
        placeholder="/path/to/fake.jsonl"
        className="pointer-coarse:h-11"
        data-testid="ai-fake-script-input"
      />
    </SettingRow>
  )
}
