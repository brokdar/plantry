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
          "group flex flex-col items-center gap-1 rounded-xl p-2 text-on-surface-variant transition-[color,background-color,box-shadow] duration-150 ease-out",
          "hover:bg-primary/10 hover:text-primary",
          "[&.active]:bg-primary/15 [&.active]:text-primary [&.active]:shadow-sm"
        )}
      >
        <Icon
          className="size-5 transition-transform duration-150 ease-out group-hover:scale-110 [.active_&]:scale-100"
          aria-hidden
        />
      </Link>
    )
  }

  return (
    <Link
      to={to}
      activeOptions={{ exact }}
      className={cn(
        "group flex items-center gap-3 rounded-r-full py-3 pr-4 pl-4 text-sm font-medium transition-[color,background-color,box-shadow] duration-150 ease-out",
        "text-on-surface-variant hover:bg-primary/10 hover:text-on-surface hover:shadow-[inset_3px_0_0_rgba(74,101,77,0.45)]",
        "[&.active]:bg-primary/15 [&.active]:font-bold [&.active]:text-primary [&.active]:shadow-[inset_3px_0_0_#4a654d]"
      )}
      data-testid={`sidenav-${to.replace(/\W+/g, "-").replace(/^-|-$/g, "") || "root"}`}
    >
      <Icon
        className="size-5 transition-transform duration-150 ease-out group-hover:scale-110 [.active_&]:scale-100"
        aria-hidden
      />
      <span>{label}</span>
    </Link>
  )
}
