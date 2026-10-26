import { Link } from "@tanstack/react-router"
import { Settings } from "lucide-react"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"

type TopBarProps = {
  leading?: React.ReactNode
  trailing?: React.ReactNode
  className?: string
}

export function TopBar({ leading, trailing, className }: TopBarProps) {
  const { t } = useTranslation()
  return (
    <header
      className={cn(
        "glass-header sticky top-0 z-30 flex h-16 items-center justify-between px-6 md:px-8",
        className
      )}
      data-testid="topbar"
    >
      <div className="flex flex-1 items-center gap-4">{leading}</div>
      <div className="flex items-center gap-2">
        {trailing}
        {/* Phone only: the rail carries Settings from 768 px up (UX §2.1). */}
        <Link
          to="/settings"
          aria-label={t("nav.settings")}
          className="flex size-11 items-center justify-center rounded-lg text-ink-muted transition-colors hover:text-ink focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none md:hidden [&.active]:text-accent"
          data-testid="topbar-settings"
        >
          <Settings className="size-5" strokeWidth={1.5} aria-hidden />
        </Link>
      </div>
    </header>
  )
}
