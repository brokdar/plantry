import { cn } from "@/lib/utils"

type DotColor = "primary" | "tertiary" | "secondary" | "muted"

const DOT_COLORS: Record<DotColor, string> = {
  primary: "bg-primary",
  tertiary: "bg-tertiary",
  secondary: "bg-secondary",
  muted: "bg-on-surface-variant/50",
}

type CardImageBadgeProps = {
  children: React.ReactNode
  dot?: DotColor
  className?: string
  testId?: string
}

export function CardImageBadge({
  children,
  dot,
  className,
  testId,
}: CardImageBadgeProps) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full bg-surface-container-lowest/85 px-2.5 py-1 text-[10px] font-medium tracking-wide text-on-surface-variant uppercase backdrop-blur-md",
        className
      )}
      data-testid={testId}
    >
      {dot && (
        <span
          aria-hidden
          className={cn("inline-block size-1.5 rounded-full", DOT_COLORS[dot])}
        />
      )}
      {children}
    </span>
  )
}
