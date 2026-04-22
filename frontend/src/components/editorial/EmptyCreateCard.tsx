import { Link } from "@tanstack/react-router"
import { PlusCircle } from "lucide-react"

import { cn } from "@/lib/utils"

type EmptyCreateCardProps = {
  to: string
  title: string
  description?: string
  className?: string
  minHeight?: string
  testId?: string
}

export function EmptyCreateCard({
  to,
  title,
  description,
  className,
  minHeight = "min-h-[300px]",
  testId = "empty-create-card",
}: EmptyCreateCardProps) {
  return (
    <Link
      to={to}
      className={cn(
        "group flex flex-col items-center justify-center rounded-xl border-2 border-dashed border-outline-variant/30 p-8 text-center transition-colors hover:border-primary/50",
        minHeight,
        className
      )}
      data-testid={testId}
    >
      <PlusCircle
        className="mb-4 size-10 text-outline-variant transition-colors group-hover:text-primary"
        aria-hidden
      />
      <p className="font-heading text-lg font-bold text-on-surface-variant group-hover:text-primary">
        {title}
      </p>
      {description && (
        <p className="mt-2 text-xs text-on-surface-variant/70">{description}</p>
      )}
    </Link>
  )
}
