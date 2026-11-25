import { Plus, X } from "lucide-react"
import { useDeferredValue, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import {
  EQUIPMENT_TOOLS,
  MAX_LEANINGS,
  MAX_LEANING_LENGTH,
  SKILLS,
  type EquipmentTool,
  type Kitchen,
  type KitchenSettings,
} from "@/lib/api/kitchen"
import { useFoods } from "@/lib/queries/foods"
import { useKitchen, usePatchKitchen, useSetBasic } from "@/lib/queries/kitchen"

import { OptionCards, SettingsGroup } from "../controls"
import { HeatPicker, MinutesPicker } from "../KitchenFields"
import { useSavedNote } from "../setting-field"
import { SettingRow } from "../SettingRow"

/** Saves a kitchen patch and flashes this row's "Saved" note. */
function useKitchenRow() {
  const patch = usePatchKitchen()
  const { saved, markSaved } = useSavedNote()
  return {
    saved,
    save: (input: Partial<KitchenSettings>) =>
      patch.mutate(input, { onSuccess: markSaved }),
  }
}

export function KitchenSection() {
  const { data: kitchen } = useKitchen()
  if (!kitchen) {
    return (
      <div className="flex flex-col gap-4" aria-hidden>
        <Skeleton className="h-16 w-full" />
        <Skeleton className="h-16 w-full" />
        <Skeleton className="h-16 w-full" />
      </div>
    )
  }
  return <KitchenFields kitchen={kitchen} />
}

function KitchenFields({ kitchen }: { kitchen: Kitchen }) {
  const { t } = useTranslation()
  const weekday = useKitchenRow()
  const weekend = useKitchenRow()
  const skill = useKitchenRow()
  const equipment = useKitchenRow()
  const heat = useKitchenRow()
  const leanings = useKitchenRow()

  return (
    <div className="flex flex-col gap-8">
      <SettingsGroup title={t("settings_page.kitchen.time_group")}>
        <SettingRow
          label={t("settings_page.kitchen.weekday_label")}
          description={t("settings_page.kitchen.weekday_effect", {
            count: kitchen.minutes_weekday,
          })}
          saved={weekday.saved}
          htmlFor="kitchen-minutes-weekday"
        >
          <MinutesPicker
            id="kitchen-minutes-weekday"
            value={kitchen.minutes_weekday}
            onChange={(minutes_weekday) => weekday.save({ minutes_weekday })}
          />
        </SettingRow>
        <SettingRow
          label={t("settings_page.kitchen.weekend_label")}
          description={t("settings_page.kitchen.weekend_effect", {
            count: kitchen.minutes_weekend,
          })}
          saved={weekend.saved}
          htmlFor="kitchen-minutes-weekend"
        >
          <MinutesPicker
            id="kitchen-minutes-weekend"
            value={kitchen.minutes_weekend}
            onChange={(minutes_weekend) => weekend.save({ minutes_weekend })}
          />
        </SettingRow>
      </SettingsGroup>

      <SettingsGroup title={t("settings_page.kitchen.kitchen_group")}>
        <SettingRow
          label={t("settings_page.kitchen.skill_label")}
          description={t(
            `settings_page.kitchen.skill_effect.${kitchen.skill ?? "unknown"}`
          )}
          saved={skill.saved}
        >
          <OptionCards
            name="kitchen-skill"
            legend={t("settings_page.kitchen.skill_label")}
            columns={3}
            options={SKILLS.map((s) => ({
              value: s,
              label: t(`settings_page.kitchen.skill.${s}`),
            }))}
            value={kitchen.skill}
            onChange={(next) => skill.save({ skill: next })}
          />
        </SettingRow>
        <SettingRow
          label={t("settings_page.kitchen.equipment_label")}
          description={t("settings_page.kitchen.equipment_effect")}
          saved={equipment.saved}
        >
          <EquipmentChecklist
            value={kitchen.equipment}
            onChange={(next) => equipment.save({ equipment: next })}
          />
        </SettingRow>
        <SettingRow
          label={t("settings_page.kitchen.heat_label")}
          description={t(
            `settings_page.kitchen.heat_effect.${kitchen.heat ?? "unknown"}`
          )}
          saved={heat.saved}
        >
          <HeatPicker
            name="kitchen-heat"
            value={kitchen.heat}
            onChange={(next) => heat.save({ heat: next })}
          />
        </SettingRow>
        <SettingRow
          label={t("settings_page.kitchen.leanings_label")}
          description={t("settings_page.kitchen.leanings_effect", {
            max: MAX_LEANINGS,
          })}
          saved={leanings.saved}
        >
          <LeaningsInput
            value={kitchen.leanings}
            onChange={(next) => leanings.save({ leanings: next })}
          />
        </SettingRow>
      </SettingsGroup>

      <SettingsGroup title={t("settings_page.kitchen.home_group")}>
        <SettingRow
          label={t("settings_page.kitchen.basics_label")}
          description={t("settings_page.kitchen.basics_effect")}
        >
          <BasicsList basics={kitchen.basics} />
        </SettingRow>
      </SettingsGroup>
    </div>
  )
}

function EquipmentChecklist({
  value,
  onChange,
}: {
  value: EquipmentTool[]
  onChange: (next: EquipmentTool[]) => void
}) {
  const { t } = useTranslation()
  return (
    <fieldset>
      <legend className="sr-only">
        {t("settings_page.kitchen.equipment_label")}
      </legend>
      <ul className="grid grid-cols-2 gap-x-4 sm:grid-cols-3">
        {EQUIPMENT_TOOLS.map((tool) => {
          const id = `kitchen-tool-${tool}`
          const present = value.includes(tool)
          return (
            <li key={tool}>
              <label
                htmlFor={id}
                className="flex min-h-11 cursor-pointer items-center gap-3 text-body text-ink"
              >
                <input
                  id={id}
                  type="checkbox"
                  checked={present}
                  onChange={() =>
                    onChange(
                      EQUIPMENT_TOOLS.filter((x) =>
                        x === tool ? !present : value.includes(x)
                      )
                    )
                  }
                  className="size-5 cursor-pointer accent-accent dark:scheme-dark"
                />
                {t(`settings_page.kitchen.tool.${tool}`)}
              </label>
            </li>
          )
        })}
      </ul>
    </fieldset>
  )
}

function LeaningsInput({
  value,
  onChange,
}: {
  value: string[]
  onChange: (next: string[]) => void
}) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState("")
  const inputRef = useRef<HTMLInputElement>(null)
  const full = value.length >= MAX_LEANINGS

  function add() {
    const trimmed = draft.trim()
    if (!trimmed || full || value.includes(trimmed)) return
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
                onClick={() => onChange(value.filter((x) => x !== tag))}
                className="flex size-6 items-center justify-center rounded-cell text-ink-muted hover:text-danger pointer-coarse:size-11"
                aria-label={t("settings_page.kitchen.leanings_remove", { tag })}
              >
                <X className="size-4" strokeWidth={1.5} aria-hidden />
              </button>
            </li>
          ))}
        </ul>
      )}
      {full ? (
        <p className="text-small text-ink-muted">
          {t("settings_page.kitchen.leanings_full", { max: MAX_LEANINGS })}
        </p>
      ) : (
        <div className="flex gap-2">
          <Input
            ref={inputRef}
            value={draft}
            maxLength={MAX_LEANING_LENGTH}
            onChange={(e) => setDraft(e.target.value)}
            placeholder={t("settings_page.kitchen.leanings_placeholder")}
            aria-label={t("settings_page.kitchen.leanings_label")}
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
            {t("settings_page.kitchen.leanings_add")}
          </Button>
        </div>
      )}
    </div>
  )
}

function BasicsList({ basics }: { basics: Kitchen["basics"] }) {
  const { t } = useTranslation()
  const setBasic = useSetBasic()
  const [search, setSearch] = useState("")
  const query = useDeferredValue(search.trim())
  const found = useFoods(
    { kind: "leaf", search: query, limit: 8 },
    { staleTime: 30_000 }
  )
  const atHome = new Set(basics.map((b) => b.id))
  const matches = (found.data?.items ?? []).filter((f) => !atHome.has(f.id))

  return (
    <div className="flex flex-col gap-3">
      {basics.length === 0 ? (
        <p className="text-small text-ink-muted">
          {t("settings_page.kitchen.basics_empty")}
        </p>
      ) : (
        <ul className="flex flex-wrap gap-2" data-testid="kitchen-basics">
          {basics.map((b) => (
            <li
              key={b.id}
              className="flex items-center gap-1 rounded-cell border border-line bg-panel py-0.5 pr-0.5 pl-2 text-body text-ink"
            >
              {b.name}
              <button
                type="button"
                onClick={() => setBasic.mutate({ id: b.id, atHome: false })}
                className="flex size-6 items-center justify-center rounded-cell text-ink-muted hover:text-danger pointer-coarse:size-11"
                aria-label={t("settings_page.kitchen.basics_remove", {
                  name: b.name,
                })}
              >
                <X className="size-4" strokeWidth={1.5} aria-hidden />
              </button>
            </li>
          ))}
        </ul>
      )}
      <Input
        type="search"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        placeholder={t("settings_page.kitchen.basics_search_placeholder")}
        aria-label={t("settings_page.kitchen.basics_search_label")}
        className="max-w-xs pointer-coarse:h-11"
      />
      {query !== "" &&
        (matches.length === 0 ? (
          !found.isPending && (
            <p className="text-small text-ink-muted">
              {t("settings_page.kitchen.basics_no_match")}
            </p>
          )
        ) : (
          <ul className="flex flex-wrap gap-2">
            {matches.map((f) => (
              <li key={f.id}>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => {
                    setBasic.mutate({ id: f.id, atHome: true })
                    setSearch("")
                  }}
                  aria-label={t("settings_page.kitchen.basics_add", {
                    name: f.name,
                  })}
                  className="pointer-coarse:h-11"
                >
                  <Plus
                    data-icon="inline-start"
                    strokeWidth={1.5}
                    aria-hidden
                  />
                  {f.name}
                </Button>
              </li>
            ))}
          </ul>
        ))}
    </div>
  )
}
