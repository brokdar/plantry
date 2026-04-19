import { Lock } from "lucide-react"
import { useTranslation } from "react-i18next"

import { SettingsCard } from "@/components/editorial/SettingsCard"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import type { SystemInfo } from "@/lib/api/settings"
import { useSystemInfo } from "@/lib/queries/settings"

export function SystemTab() {
  const { t } = useTranslation()
  const { data, isLoading } = useSystemInfo()

  return (
    <SettingsCard title={t("settings_page.tabs.system")}>
      <p className="text-sm leading-relaxed text-on-surface-variant">
        {t("settings_page.system.readonly_notice")}
      </p>
      {isLoading || !data ? (
        <div className="space-y-3">
          <Skeleton className="h-6 w-full" />
          <Skeleton className="h-6 w-full" />
          <Skeleton className="h-6 w-3/4" />
        </div>
      ) : (
        <SystemTable data={data} />
      )}
    </SettingsCard>
  )
}

const ROWS: Array<{
  field: keyof Omit<SystemInfo, "cipher_available">
  envVar: string
}> = [
  { field: "port", envVar: "PLANTRY_PORT" },
  { field: "db_path", envVar: "PLANTRY_DB_PATH" },
  { field: "log_level", envVar: "PLANTRY_LOG_LEVEL" },
  { field: "image_path", envVar: "PLANTRY_IMAGE_PATH" },
  { field: "dev_mode", envVar: "PLANTRY_DEV_MODE" },
  { field: "version", envVar: "" },
  { field: "build_commit", envVar: "" },
]

function SystemTable({ data }: { data: SystemInfo }) {
  const { t } = useTranslation()
  return (
    <div className="overflow-hidden rounded-xl border border-outline/40">
      {ROWS.map(({ field, envVar }, idx) => {
        const value = data[field]
        const display =
          value === true
            ? t("common.yes")
            : value === false
              ? t("common.no")
              : String(value ?? "—")
        return (
          <div
            key={field}
            className={
              "grid grid-cols-[minmax(0,1fr)_minmax(0,2fr)] items-center gap-4 border-outline/40 px-4 py-3 text-sm" +
              (idx > 0 ? " border-t" : "")
            }
          >
            <div className="flex items-center gap-2">
              <span className="font-medium text-on-surface">
                {t(`settings_page.system.fields.${field}`)}
              </span>
              {envVar && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Lock
                      className="size-3.5 text-on-surface-variant"
                      aria-label={t("settings_page.system.restart_required")}
                    />
                  </TooltipTrigger>
                  <TooltipContent className="max-w-xs text-xs">
                    {t("settings_page.system.env_tooltip", { envVar })}
                  </TooltipContent>
                </Tooltip>
              )}
            </div>
            <code className="font-mono text-xs break-all text-on-surface-variant">
              {display}
            </code>
          </div>
        )
      })}
      {!data.cipher_available && (
        <div className="border-t border-outline/40 bg-tertiary/5 px-4 py-3 text-xs">
          <p className="font-medium text-on-surface">
            {t("settings_page.system.secret_key_banner_title")}
          </p>
          <p className="mt-1 leading-relaxed text-on-surface-variant">
            {t("settings_page.system.secret_key_banner_body")}
          </p>
        </div>
      )}
    </div>
  )
}
