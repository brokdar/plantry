import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import type { SettingSource } from "@/lib/api/settings"
import { cn } from "@/lib/utils"

type SourceBadgeProps = {
  source: SettingSource
  envAlsoSet?: boolean
  className?: string
}

const VARIANT_STYLES: Record<SettingSource, string> = {
  db: "border-accent/30 bg-accent/10 text-accent",
  env: "border-line bg-panel-sunken text-ink",
  default: "border-line bg-transparent text-ink-muted",
}

/**
 * SourceBadge tells the user whether a setting's current value is coming
 * from the database (user override), an environment variable, or the built-in
 * default. When a DB override is active and an env var is also set, a small
 * hint surfaces that the DB value wins.
 */
export function SourceBadge({
  source,
  envAlsoSet,
  className,
}: SourceBadgeProps) {
  const { t } = useTranslation()
  const label = t(`settings_page.source.${source}`)
  const tooltip =
    source === "db" && envAlsoSet
      ? t("settings_page.source.env_also_set")
      : t(`settings_page.source.tooltip_${source}`)

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge
          variant="outline"
          className={cn(
            "rounded-cell text-caption font-normal",
            VARIANT_STYLES[source],
            className
          )}
          data-testid={`source-badge-${source}`}
        >
          {label}
        </Badge>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs text-xs">{tooltip}</TooltipContent>
    </Tooltip>
  )
}
