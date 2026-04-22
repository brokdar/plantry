import { useEffect } from "react"

import { cn } from "@/lib/utils"

export type FilterChipProps = {
  selected?: boolean
  onClick?: () => void
  children: React.ReactNode
  className?: string
  testId?: string
  ariaLabel?: string
}

export function FilterChip({
  selected,
  onClick,
  children,
  className,
  testId,
  ariaLabel,
}: FilterChipProps) {
  useEffect(() => {
    if (!import.meta.env.DEV) return
    if (typeof children !== "string" && !ariaLabel) {
      console.warn(
        "[FilterChip] non-text children must be accompanied by ariaLabel for screen readers."
      )
    }
  }, [children, ariaLabel])

  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={selected}
      aria-label={ariaLabel}
      data-testid={testId}
      className={cn(
        "rounded-full px-5 py-2 text-sm font-semibold transition-all active:scale-95",
        selected
          ? "bg-primary text-on-primary shadow-sm"
          : "bg-secondary-container text-on-secondary-container hover:bg-surface-container-highest",
        className
      )}
    >
      {children}
    </button>
  )
}
