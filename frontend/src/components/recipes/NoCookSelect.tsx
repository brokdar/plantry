import { useTranslation } from "react-i18next"

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
} from "@/components/ui/select"
import { cn } from "@/lib/utils"

const AUTO = "auto"
const YES = "yes"
const NO = "no"

type NoCookSelectProps = {
  /** The user's override; null = automatic. */
  value: boolean | null
  /** What automatic currently means; unknown before the food exists. */
  effective?: boolean
  onChange: (next: boolean | null) => void
  disabled?: boolean
  className?: string
  testId?: string
}

/** The "needs no cooking" tri-state: automatic (the planner guesses), yes or
 *  no. */
export function NoCookSelect({
  value,
  effective,
  onChange,
  disabled,
  className,
  testId,
}: NoCookSelectProps) {
  const { t } = useTranslation()
  const current = value === null ? AUTO : value ? YES : NO
  const autoHint =
    value === null && effective !== undefined
      ? effective
        ? t("no_cook.yes")
        : t("no_cook.no")
      : null

  return (
    <Select
      value={current}
      onValueChange={(v) => onChange(v === AUTO ? null : v === YES)}
      disabled={disabled}
    >
      <SelectTrigger
        aria-label={t("no_cook.label")}
        className={cn(
          "min-h-11 rounded-cell border-line bg-transparent text-body data-[size=default]:h-auto md:pointer-fine:min-h-8",
          className
        )}
        data-testid={testId}
      >
        <span className="flex items-center gap-1.5 text-left">
          {current === AUTO
            ? t("no_cook.auto")
            : current === YES
              ? t("no_cook.yes")
              : t("no_cook.no")}
          {autoHint && (
            <span className="text-caption text-ink-muted">
              {t("no_cook.auto_hint")}: {autoHint}
            </span>
          )}
        </span>
      </SelectTrigger>
      <SelectContent position="popper">
        <SelectItem value={AUTO}>{t("no_cook.auto")}</SelectItem>
        <SelectItem value={YES}>{t("no_cook.yes")}</SelectItem>
        <SelectItem value={NO}>{t("no_cook.no")}</SelectItem>
      </SelectContent>
    </Select>
  )
}
