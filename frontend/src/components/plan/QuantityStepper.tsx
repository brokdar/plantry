import { Minus, Plus } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { formatAmount } from "@/lib/plan/format"
import { cn } from "@/lib/utils"

interface QuantityStepperProps {
  /** Names the value: the group's label and the − / + buttons. */
  label: string
  value: number
  min: number
  max: number
  step: number
  onChange: (next: number) => void
  /** Shows the value; defaults to fractions ("1½"). */
  format?: (value: number) => string
  disabled?: boolean
  className?: string
  testId?: string
}

/** Snaps to the step grid, so 0.25 steps never drift into 0.7500000001. */
function snap(value: number, step: number) {
  return Math.round(value / step) * step
}

/**
 * − value + for real quantities: portions per eater (step ¼), eaters
 * (step ½), meals and batch portions (step 1). Buttons are 44 px on touch
 * and shrink only for a fine pointer from 768 px.
 */
export function QuantityStepper({
  label,
  value,
  min,
  max,
  step,
  onChange,
  format,
  disabled,
  className,
  testId,
}: QuantityStepperProps) {
  const { t, i18n } = useTranslation()
  const shown = format ? format(value) : formatAmount(value, i18n)

  function set(next: number) {
    onChange(Math.min(max, Math.max(min, snap(next, step))))
  }

  return (
    <div
      role="group"
      aria-label={label}
      data-testid={testId}
      className={cn("flex items-center gap-1", className)}
    >
      <Button
        type="button"
        variant="outline"
        className="size-11 md:pointer-fine:size-9"
        aria-label={t("plan.stepper.decrease", { label })}
        disabled={disabled || value <= min}
        onClick={() => set(value - step)}
      >
        <Minus strokeWidth={1.5} aria-hidden />
      </Button>
      <output
        aria-live="polite"
        className="min-w-12 text-center text-body text-ink tabular-nums"
      >
        {shown}
      </output>
      <Button
        type="button"
        variant="outline"
        className="size-11 md:pointer-fine:size-9"
        aria-label={t("plan.stepper.increase", { label })}
        disabled={disabled || value >= max}
        onClick={() => set(value + step)}
      >
        <Plus strokeWidth={1.5} aria-hidden />
      </Button>
    </div>
  )
}
