import { Minus, Plus } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

const MIN_PORTIONS = 1
const MAX_PORTIONS = 20

/**
 * PortionStepper drives the integer "× n portions" input for *composed*
 * plate components (recipes). Pure integers only — no fractional steps,
 * no `×0.25` heritage. Composed quantities map 1:1 onto recipe servings;
 * fractional servings are not a thing the rest of the system understands.
 *
 * For *leaf* plate components, use `QuantityUnitInput` instead — they share
 * the same call site (the picker tray and the slot sheet) but have
 * deliberately separate component types so neither can wander into the
 * other's input shape.
 */
interface PortionStepperProps {
  value: number
  onChange: (next: number) => void
  /** Visual scale; "sm" suits a tray row, "md" is the default. */
  size?: "sm" | "md"
  className?: string
}

export function PortionStepper({
  value,
  onChange,
  size = "md",
  className,
}: PortionStepperProps) {
  const { t } = useTranslation()
  const v = Math.max(MIN_PORTIONS, Math.round(value))
  const canDec = v > MIN_PORTIONS
  const canInc = v < MAX_PORTIONS

  const buttonSize = size === "sm" ? "size-6" : "size-7"
  const valueSize =
    size === "sm" ? "min-w-[1.6rem] text-[11.5px]" : "min-w-[2rem] text-[13px]"

  return (
    <div
      className={cn(
        "inline-flex items-center gap-0 rounded-full border border-outline-variant/50 bg-surface-container-lowest",
        className
      )}
      role="group"
      aria-label={t("plate.quantity.portions_label")}
    >
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label={`${t("plate.quantity.portions_label")} −1`}
        onClick={() => canDec && onChange(v - 1)}
        disabled={!canDec}
        className={cn(buttonSize, "rounded-full text-on-surface-variant")}
      >
        <Minus className="h-3 w-3" />
      </Button>
      <span
        className={cn(
          "text-center font-mono font-semibold text-on-surface tabular-nums",
          valueSize
        )}
        role="spinbutton"
        aria-valuenow={v}
        aria-valuemin={MIN_PORTIONS}
        aria-valuemax={MAX_PORTIONS}
        data-testid="portion-stepper-value"
      >
        ×{v}
      </span>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label={`${t("plate.quantity.portions_label")} +1`}
        onClick={() => canInc && onChange(v + 1)}
        disabled={!canInc}
        className={cn(buttonSize, "rounded-full text-on-surface-variant")}
      >
        <Plus className="h-3 w-3" />
      </Button>
    </div>
  )
}
