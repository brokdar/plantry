import { X } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import type { SettingItem } from "@/lib/api/settings"
import { cn } from "@/lib/utils"

import { SavedNote } from "./controls"
import { SourceBadge } from "./SourceBadge"

type SettingRowProps = {
  label: React.ReactNode
  /** One line saying what the current value does. */
  description?: React.ReactNode
  item?: SettingItem
  children: React.ReactNode
  onClearOverride?: () => void
  /** Shows the quiet "Saved" note after a successful save. */
  saved?: boolean
  /** id of the control, so the label names it. */
  htmlFor?: string
  className?: string
}

/**
 * SettingRow renders a single editable setting: a label, the one-line effect
 * of its current value, the control (children), the source badge, and a
 * "clear override" action when a database override is active.
 */
export function SettingRow({
  label,
  description,
  item,
  children,
  onClearOverride,
  saved,
  htmlFor,
  className,
}: SettingRowProps) {
  const { t } = useTranslation()
  const canClear = item?.source === "db" && !!onClearOverride

  return (
    <div
      className={cn(
        "grid gap-3 border-b border-line py-5 last:border-b-0 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)] lg:gap-8",
        className
      )}
    >
      <div className="flex flex-col gap-1">
        <div className="flex flex-wrap items-center gap-2">
          {htmlFor ? (
            <label
              htmlFor={htmlFor}
              className="text-body font-semibold text-ink"
            >
              {label}
            </label>
          ) : (
            <p className="text-body font-semibold text-ink">{label}</p>
          )}
          {item && (
            <SourceBadge source={item.source} envAlsoSet={item.env_also_set} />
          )}
          <SavedNote saved={!!saved} />
        </div>
        {description && (
          <p className="text-small text-ink-muted">{description}</p>
        )}
        {canClear && (
          <Button
            variant="ghost"
            size="sm"
            type="button"
            className="self-start px-2 text-ink-muted"
            onClick={onClearOverride}
            data-testid={`clear-override-${item.key}`}
          >
            <X data-icon="inline-start" strokeWidth={1.5} aria-hidden />
            {t("settings_page.clear_override")}
          </Button>
        )}
      </div>
      <div className="flex min-w-0 flex-col gap-2">{children}</div>
    </div>
  )
}
