import { cn } from "@/lib/utils"

type SettingsCardProps = {
  title?: React.ReactNode
  icon?: React.ReactNode
  children: React.ReactNode
  className?: string
  tonal?: "low" | "lowest"
}

export function SettingsCard({
  title,
  icon,
  children,
  className,
  tonal = "lowest",
}: SettingsCardProps) {
  return (
    <section
      className={cn(
        "space-y-6 rounded-2xl p-8",
        tonal === "low"
          ? "bg-surface-container-low"
          : "editorial-shadow bg-surface-container-lowest",
        className
      )}
    >
      {title && (
        <header className="flex items-center gap-3">
          {icon && (
            <span className="text-primary" aria-hidden>
              {icon}
            </span>
          )}
          <h3 className="font-heading text-xl font-bold text-on-surface">
            {title}
          </h3>
        </header>
      )}
      {children}
    </section>
  )
}
