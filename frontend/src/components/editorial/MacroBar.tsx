import { cn } from "@/lib/utils"

type MacroBarSegment = {
  value: number
  color?: "primary" | "tertiary" | "secondary" | "accent"
  label?: string
}

type MacroBarProps = {
  value?: number
  max?: number
  segments?: MacroBarSegment[]
  track?: "primary-fixed" | "tertiary-fixed" | "surface-container-highest"
  thickness?: "sm" | "md" | "lg"
  className?: string
}

const FILL_COLORS: Record<NonNullable<MacroBarSegment["color"]>, string> = {
  primary: "bg-primary",
  tertiary: "bg-tertiary",
  secondary: "bg-outline",
  accent: "bg-primary-container",
}

const TRACK_COLORS: Record<NonNullable<MacroBarProps["track"]>, string> = {
  "primary-fixed": "bg-primary-fixed",
  "tertiary-fixed": "bg-tertiary-fixed-dim",
  "surface-container-highest": "bg-surface-container-highest",
}

const THICKNESS: Record<NonNullable<MacroBarProps["thickness"]>, string> = {
  sm: "h-1",
  md: "h-1.5",
  lg: "h-2",
}

export function MacroBar({
  value,
  max = 100,
  segments,
  track = "primary-fixed",
  thickness = "md",
  className,
}: MacroBarProps) {
  const effectiveSegments =
    segments ??
    (value !== undefined ? [{ value, color: "primary" as const }] : [])

  const total = Math.max(
    max,
    effectiveSegments.reduce((sum, s) => sum + s.value, 0)
  )

  return (
    <div
      role="progressbar"
      aria-valuemin={0}
      aria-valuemax={total}
      aria-valuenow={effectiveSegments[0]?.value}
      className={cn(
        "flex w-full overflow-hidden rounded-full",
        TRACK_COLORS[track],
        THICKNESS[thickness],
        className
      )}
      data-testid="macro-bar"
    >
      {effectiveSegments.map((segment, idx) => {
        const pct = Math.min(100, (segment.value / total) * 100)
        return (
          <div
            key={`${segment.label ?? idx}`}
            className={cn("h-full", FILL_COLORS[segment.color ?? "primary"])}
            style={{ width: `${pct}%` }}
            aria-hidden
            title={segment.label}
          />
        )
      })}
    </div>
  )
}
