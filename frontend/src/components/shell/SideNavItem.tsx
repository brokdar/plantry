import { Link } from "@tanstack/react-router"
import type { LucideIcon } from "lucide-react"

import { cn } from "@/lib/utils"

type SideNavItemProps = {
  to: string
  label: string
  icon: LucideIcon
  exact?: boolean
  variant?: "default" | "rail"
}

export function SideNavItem({
  to,
  label,
  icon: Icon,
  exact,
  variant = "default",
}: SideNavItemProps) {
  if (variant === "rail") {
    return (
      <Link
        to={to}
        activeOptions={{ exact }}
        className={cn(
          "group flex flex-col items-center gap-1 rounded-lg p-2 text-on-surface-variant transition-all",
          "hover:bg-surface-container-high",
          "[&.active]:bg-surface-container-lowest [&.active]:font-bold [&.active]:text-primary [&.active]:shadow-sm"
        )}
      >
        <Icon className="size-5" aria-hidden />
      </Link>
    )
  }

  return (
    <Link
      to={to}
      activeOptions={{ exact }}
      className={cn(
        "group flex items-center gap-3 rounded-r-full py-3 pr-4 pl-4 text-sm font-medium transition-all",
        "text-on-surface-variant hover:translate-x-1 hover:bg-surface-container-high",
        "[&.active]:bg-surface-container-lowest [&.active]:font-bold [&.active]:text-primary [&.active]:shadow-sm"
      )}
      data-testid={`sidenav-${to.replace(/\W+/g, "-").replace(/^-|-$/g, "") || "root"}`}
    >
      <Icon className="size-5" aria-hidden />
      <span>{label}</span>
    </Link>
  )
}
