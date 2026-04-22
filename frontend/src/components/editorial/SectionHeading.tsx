import { cn } from "@/lib/utils"

type SectionHeadingProps = {
  eyebrow?: React.ReactNode
  title: React.ReactNode
  description?: React.ReactNode
  trailing?: React.ReactNode
  className?: string
  size?: "md" | "lg"
}

export function SectionHeading({
  eyebrow,
  title,
  description,
  trailing,
  className,
  size = "md",
}: SectionHeadingProps) {
  return (
    <div
      className={cn(
        "flex flex-col justify-between gap-2 sm:flex-row sm:items-end",
        className
      )}
    >
      <div className="space-y-1">
        {eyebrow && (
          <span className="text-xs font-bold tracking-widest text-primary uppercase">
            {eyebrow}
          </span>
        )}
        <h2
          className={cn(
            "font-heading font-bold text-on-surface",
            size === "md" ? "text-xl" : "text-2xl md:text-3xl"
          )}
        >
          {title}
        </h2>
        {description && (
          <p className="text-sm text-on-surface-variant">{description}</p>
        )}
      </div>
      {trailing && <div className="flex items-center gap-2">{trailing}</div>}
    </div>
  )
}
