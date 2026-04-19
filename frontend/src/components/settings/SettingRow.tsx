import { X } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import type { SettingItem } from "@/lib/api/settings"
import { cn } from "@/lib/utils"

import { SourceBadge } from "./SourceBadge"

type SettingRowProps = {
  label: React.ReactNode
  description?: React.ReactNode
  item?: SettingItem
  children: React.ReactNode
  onClearOverride?: () => void
  className?: string
}

/**
 * SettingRow renders a single editable setting: a label, its description, the
 * input control (provided via children), and the source badge plus an
 * optional "clear override" action when a database override is active.
 */
export function SettingRow({
  label,
  description,
  item,
  children,
  onClearOverride,
  className,
}: SettingRowProps) {
  const { t } = useTranslation()
  const canClear = item?.source === "db" && !!onClearOverride

  return (
    <div
      className={cn(
        "grid gap-4 border-b border-outline/50 py-5 last:border-b-0 md:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)] md:gap-8",
        className
      )}
    >
      <div className="space-y-1">
        <div className="flex items-center gap-2">
          <p className="font-heading text-sm font-semibold tracking-wide text-on-surface">
            {label}
          </p>
          {item && (
            <SourceBadge source={item.source} envAlsoSet={item.env_also_set} />
          )}
        </div>
        {description && (
          <p className="text-xs leading-relaxed text-on-surface-variant">
            {description}
          </p>
        )}
        {canClear && (
          <Button
            variant="ghost"
            size="sm"
            type="button"
            className="h-auto px-2 py-1 text-[0.7rem] tracking-wider text-on-surface-variant uppercase"
            onClick={onClearOverride}
            data-testid={`clear-override-${item.key}`}
          >
            <X className="mr-1 size-3" aria-hidden />
            {t("settings_page.clear_override")}
          </Button>
        )}
      </div>
      <div className="flex flex-col gap-2">{children}</div>
    </div>
  )
}
