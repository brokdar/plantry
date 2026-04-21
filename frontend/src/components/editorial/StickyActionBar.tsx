import { cn } from "@/lib/utils"

type StickyActionBarProps = {
  primary: React.ReactNode
  secondary?: React.ReactNode
  destructive?: React.ReactNode
  hint?: React.ReactNode
  className?: string
  testId?: string
}

export function StickyActionBar({
  primary,
  secondary,
  destructive,
  hint,
  className,
  testId,
}: StickyActionBarProps) {
  return (
    <div
      className={cn(
        "sticky z-30 -mx-4 mt-4 border-t border-outline-variant/30 bg-surface-container-lowest px-4 py-3 shadow-[0_-8px_24px_-12px_rgba(0,0,0,0.22)] md:-mx-8 md:px-8",
        "bottom-[var(--tab-bar-inset)] md:bottom-0",
        "[padding-bottom:max(0.75rem,env(safe-area-inset-bottom))] md:[padding-bottom:0.75rem]",
        className
      )}
      data-testid={testId ?? "sticky-action-bar"}
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        {hint ? (
          <p className="text-xs text-on-surface-variant">{hint}</p>
        ) : (
          <span aria-hidden />
        )}
        <div className="flex flex-wrap items-center gap-2">
          {destructive}
          {secondary}
          {primary}
        </div>
      </div>
    </div>
  )
}
