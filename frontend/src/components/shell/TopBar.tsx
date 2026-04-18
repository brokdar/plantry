import { Moon, Sun } from "lucide-react"
import { useTranslation } from "react-i18next"

import { useTheme } from "@/components/theme-provider"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

type TopBarProps = {
  leading?: React.ReactNode
  trailing?: React.ReactNode
  className?: string
}

export function TopBar({ leading, trailing, className }: TopBarProps) {
  const { t } = useTranslation()
  const { theme, setTheme } = useTheme()

  const isDark =
    theme === "dark" ||
    (theme === "system" &&
      typeof window !== "undefined" &&
      window.matchMedia("(prefers-color-scheme: dark)").matches)

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
        <Button
          variant="ghost"
          size="icon"
          onClick={() => setTheme(isDark ? "light" : "dark")}
          aria-label={isDark ? "Switch to light theme" : "Switch to dark theme"}
          title={t("nav.brand")}
          role="switch"
          aria-checked={isDark}
          data-testid="theme-toggle"
        >
          {isDark ? (
            <Sun className="size-4" aria-hidden />
          ) : (
            <Moon className="size-4" aria-hidden />
          )}
        </Button>
      </div>
    </header>
  )
}
