import { cn } from "@/lib/utils"

type PageHeaderProps = {
  eyebrow?: React.ReactNode
  title: React.ReactNode
  description?: React.ReactNode
  actions?: React.ReactNode
  breadcrumb?: React.ReactNode
  className?: string
}

export function PageHeader({
  eyebrow,
  title,
  description,
  actions,
  breadcrumb,
  className,
}: PageHeaderProps) {
  return (
    <header
      className={cn(
        "mb-12 flex flex-col justify-between gap-6 md:flex-row md:items-end",
        className
      )}
      data-testid="page-header"
    >
      <div className="max-w-2xl space-y-2">
        {breadcrumb && (
          <nav className="flex items-center gap-2 text-sm text-on-surface-variant">
            {breadcrumb}
          </nav>
        )}
        {eyebrow && (
          <span className="font-body text-xs font-bold tracking-widest text-primary uppercase">
            {eyebrow}
          </span>
        )}
        <h1 className="font-heading text-4xl leading-tight font-extrabold tracking-tight text-on-surface md:text-5xl">
          {title}
        </h1>
        {description && (
          <p className="max-w-lg text-base leading-relaxed text-on-surface-variant md:text-lg">
            {description}
          </p>
        )}
      </div>
      {actions && (
        <div className="flex flex-wrap items-center gap-3">{actions}</div>
      )}
    </header>
  )
}
