import { Link } from "@tanstack/react-router"
import type { LucideIcon } from "lucide-react"

import { cn } from "@/lib/utils"

type SideNavItemProps = {
  to: string
  label: string
  icon: LucideIcon
  active: boolean
  variant?: "default" | "rail"
}

export function SideNavItem({
  to,
  label,
  icon: Icon,
  active,
  variant = "default",
}: SideNavItemProps) {
  const testId = `sidenav-${to.replace(/\W+/g, "-").replace(/^-|-$/g, "") || "root"}`

  return (
    <Link
      to={to}
      activeOptions={{ exact: true }}
      aria-current={active ? "page" : undefined}
      data-active={active}
      className={cn(
        "flex text-ink-muted transition-colors duration-150 ease-out",
        "hover:bg-panel hover:text-ink",
        "focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none",
        "data-[active=true]:bg-accent/10 data-[active=true]:font-semibold data-[active=true]:text-accent",
        variant === "rail"
          ? "flex-col items-center gap-1 rounded-lg px-1 py-2 text-xs"
          : "items-center gap-3 rounded-lg px-4 py-2.5 text-sm"
      )}
      data-testid={testId}
    >
      <Icon className="size-5 shrink-0" strokeWidth={1.5} aria-hidden />
      <span>{label}</span>
    </Link>
  )
}
