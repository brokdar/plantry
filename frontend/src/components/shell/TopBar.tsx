import { cn } from "@/lib/utils"

type TopBarProps = {
  leading?: React.ReactNode
  trailing?: React.ReactNode
  className?: string
}

export function TopBar({ leading, trailing, className }: TopBarProps) {
  return (
    <header
      className={cn(
        "glass-header sticky top-0 z-30 flex h-16 items-center justify-between px-6 md:px-8",
        className
      )}
      data-testid="topbar"
    >
      <div className="flex flex-1 items-center gap-4">{leading}</div>
      <div className="flex items-center gap-2">{trailing}</div>
    </header>
  )
}
