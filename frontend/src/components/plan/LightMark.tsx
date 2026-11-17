import { Feather } from "lucide-react"

import { cn } from "@/lib/utils"

/**
 * A day's light request: a small feather. On the phone list the label shows
 * beside it; on the grid it is the icon with the label as its name and
 * tooltip, so the mark never relies on colour.
 */
export function LightMark({
  label,
  showLabel,
  className,
}: {
  label: string
  showLabel?: boolean
  className?: string
}) {
  return (
    <span
      data-testid="light-mark"
      title={label}
      className={cn(
        "inline-flex items-center gap-1 text-caption text-ink-muted",
        className
      )}
    >
      <Feather className="size-3.5 shrink-0" aria-hidden="true" />
      {showLabel ? label : <span className="sr-only">{label}</span>}
    </span>
  )
}
