import { Link } from "@tanstack/react-router"
import { ArrowRight, Lock } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import type { SystemInfo } from "@/lib/api/settings"
import { useDebugWorkflow } from "@/lib/debugWorkflow"
import { useSystemInfo } from "@/lib/queries/settings"

import { DisplayPreferences } from "../DisplayPreferences"
import { LanguagePreference } from "../LanguagePreference"
import { SettingRow } from "../SettingRow"
import { SettingsGroup, Switch } from "../controls"
import { useSavedNote, useSettingField } from "../setting-field"

export function SystemSection() {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col gap-10">
      <SettingsGroup title={t("settings_page.system_section.appearance_group")}>
        <LanguagePreference />
        <DisplayPreferences />
      </SettingsGroup>

      <SettingsGroup title={t("settings_page.system_section.data_group")}>
        <FDCKeyRow />
        <SettingRow
          label={t("settings_page.system_section.setup_label")}
          description={t("settings_page.system_section.setup_effect")}
        >
          <Button
            asChild
            variant="outline"
            className="self-start pointer-coarse:h-11"
          >
            <Link to="/setup" data-testid="rerun-setup">
              {t("settings_page.system_section.setup_link")}
              <ArrowRight
                data-icon="inline-end"
                strokeWidth={1.5}
                aria-hidden
              />
            </Link>
          </Button>
        </SettingRow>
        <DebugRow />
      </SettingsGroup>

      <SettingsGroup
        title={t("settings_page.system_section.about_group")}
        description={t("settings_page.system.readonly_notice")}
      >
        <About />
      </SettingsGroup>
    </div>
  )
}

/** USDA FoodData Central key (moved from the AI tab). Secret, so it saves
 *  on its own button rather than on blur. */
function FDCKeyRow() {
  const { t } = useTranslation()
  const field = useSettingField("fdc.api_key", "")
  const [draft, setDraft] = useState("")
  const item = field.item
  const hasStored = !!item && (item.source === "db" || item.source === "env")

  return (
    <SettingRow
      label={t("settings_page.fdc.api_key_label")}
      description={
        hasStored
          ? t("settings_page.fdc.api_key_stored", {
              preview: item?.masked_preview ?? "",
            })
          : t("settings_page.fdc.api_key_description")
      }
      item={item}
      onClearOverride={field.clear}
      saved={field.saved}
      htmlFor="setting-fdc-key"
    >
      <div className="flex gap-3">
        <Input
          id="setting-fdc-key"
          type="password"
          autoComplete="off"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder={
            hasStored
              ? t("settings_page.fdc.api_key_placeholder_replace")
              : t("settings_page.fdc.api_key_placeholder")
          }
          className="pointer-coarse:h-11"
          data-testid="fdc-key-input"
        />
        <Button
          type="button"
          variant="outline"
          className="pointer-coarse:h-11"
          onClick={() => {
            if (!draft) return
            field.save(draft)
            setDraft("")
          }}
          disabled={!draft}
        >
          {t("common.save")}
        </Button>
      </div>
    </SettingRow>
  )
}

/** Lookup pipeline trace: a per-browser developer flag. */
function DebugRow() {
  const { t } = useTranslation()
  const [debug, setDebug] = useDebugWorkflow()
  const { saved, markSaved } = useSavedNote()
  return (
    <SettingRow
      label={t("developer_preferences.debug_title")}
      description={t(
        debug
          ? "settings_page.system_section.debug_effect_on"
          : "settings_page.system_section.debug_effect_off"
      )}
      saved={saved}
      htmlFor="setting-debug-workflow"
    >
      <Switch
        id="setting-debug-workflow"
        checked={debug}
        onChange={(next) => {
          setDebug(next)
          markSaved()
        }}
        testId="debug-workflow-toggle"
      />
    </SettingRow>
  )
}

const ROWS: Array<{
  field: keyof Omit<SystemInfo, "cipher_available">
  envVar: string
}> = [
  { field: "version", envVar: "" },
  { field: "build_commit", envVar: "" },
  { field: "port", envVar: "PLANTRY_PORT" },
  { field: "db_path", envVar: "PLANTRY_DB_PATH" },
  { field: "log_level", envVar: "PLANTRY_LOG_LEVEL" },
  { field: "image_path", envVar: "PLANTRY_IMAGE_PATH" },
  { field: "dev_mode", envVar: "PLANTRY_DEV_MODE" },
]

/** About / version: read-only infrastructure values (from the System tab). */
function About() {
  const { t } = useTranslation()
  const { data } = useSystemInfo()

  if (!data) {
    return (
      <div className="flex flex-col gap-3 pt-4" aria-hidden>
        <Skeleton className="h-6 w-full" />
        <Skeleton className="h-6 w-full" />
        <Skeleton className="h-6 w-3/4" />
      </div>
    )
  }

  return (
    <dl className="flex flex-col" data-testid="system-info">
      {ROWS.map(({ field, envVar }) => {
        const value = data[field]
        const display =
          value === true
            ? t("common.yes")
            : value === false
              ? t("common.no")
              : String(value ?? "") || "—"
        return (
          <div
            key={field}
            className="grid grid-cols-[minmax(0,1fr)_minmax(0,2fr)] items-center gap-4 border-b border-line py-3 last:border-b-0"
          >
            <dt className="flex items-center gap-2 text-body text-ink">
              {t(`settings_page.system.fields.${field}`)}
              {envVar && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Lock
                      className="size-4 text-ink-muted"
                      strokeWidth={1.5}
                      aria-label={t("settings_page.system.env_tooltip", {
                        envVar,
                      })}
                    />
                  </TooltipTrigger>
                  <TooltipContent className="max-w-xs">
                    {t("settings_page.system.env_tooltip", { envVar })}
                  </TooltipContent>
                </Tooltip>
              )}
            </dt>
            <dd className="font-mono text-small break-all text-ink-muted">
              {display}
            </dd>
          </div>
        )
      })}
    </dl>
  )
}
