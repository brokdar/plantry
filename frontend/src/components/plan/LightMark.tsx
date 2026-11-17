import { Feather, Flame } from "lucide-react"

import { cn } from "@/lib/utils"

/**
 * A day's light request: a small feather (a flame for a hearty one). On the phone list the label shows
 * beside it; on the grid it is the icon with the label as its name and
 * tooltip, so the mark never relies on colour.
 */
export function LightMark({
  label,
  hearty,
  showLabel,
  className,
}: {
  label: string
  hearty?: boolean
  showLabel?: boolean
  className?: string
}) {
  const Icon = hearty ? Flame : Feather
  return (
    <span
      data-testid="light-mark"
      title={label}
      className={cn(
        "inline-flex items-center gap-1 text-caption text-ink-muted",
        className
      )}
    >
      <Icon className="size-3.5 shrink-0" aria-hidden="true" />
      {showLabel ? label : <span className="sr-only">{label}</span>}
    </span>
  )
}
