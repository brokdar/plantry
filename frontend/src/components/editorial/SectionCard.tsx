import { cn } from "@/lib/utils"

type SectionCardProps = {
  title?: React.ReactNode
  description?: React.ReactNode
  actions?: React.ReactNode
  children: React.ReactNode
  className?: string
  bodyClassName?: string
  testId?: string
}

export function SectionCard({
  title,
  description,
  actions,
  children,
  className,
  bodyClassName,
  testId,
}: SectionCardProps) {
  return (
    <section
      className={cn(
        "editorial-shadow rounded-2xl bg-surface-container-lowest p-6 md:p-8",
        className
      )}
      data-testid={testId}
    >
      {(title || description || actions) && (
        <header className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          <div className="space-y-1">
            {title && (
              <h2 className="font-body text-xs font-bold tracking-widest text-primary uppercase">
                {title}
              </h2>
            )}
            {description && (
              <p className="text-sm text-on-surface-variant">{description}</p>
            )}
          </div>
          {actions && (
            <div className="flex flex-wrap items-center gap-2">{actions}</div>
          )}
        </header>
      )}
      <div className={cn("space-y-6", bodyClassName)}>{children}</div>
    </section>
  )
}
