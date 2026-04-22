import {
  BookOpen,
  CalendarDays,
  History,
  Package,
  Settings,
} from "lucide-react"
import type { LucideIcon } from "lucide-react"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"
import { GeneratePlanButton } from "./GeneratePlanButton"

type MobileNavEntry = {
  to: string
  labelKey: string
  icon: LucideIcon
  exact?: boolean
}

const LEFT_ENTRIES: MobileNavEntry[] = [
  { to: "/", labelKey: "nav.planner", icon: CalendarDays, exact: true },
  { to: "/components", labelKey: "nav.recipes", icon: BookOpen },
]

const RIGHT_ENTRIES: MobileNavEntry[] = [
  { to: "/ingredients", labelKey: "nav.ingredients", icon: Package },
  { to: "/settings", labelKey: "nav.settings", icon: Settings },
]

const MORE_ENTRY: MobileNavEntry = {
  to: "/archive",
  labelKey: "nav.archive",
  icon: History,
}

export function MobileBottomNav() {
  const { t } = useTranslation()

  return (
    <nav
      className={cn(
        "glass-header fixed bottom-0 left-0 z-40 flex w-full items-center justify-between",
        "border-t border-outline-variant/15 px-6 py-3 md:hidden"
      )}
      data-testid="mobile-bottom-nav"
    >
      {LEFT_ENTRIES.map((item) => (
        <BottomNavLink key={item.to} item={item} t={t} />
      ))}
      <div className="relative -top-6">
        <GeneratePlanButton variant="fab" />
      </div>
      {RIGHT_ENTRIES.map((item) => (
        <BottomNavLink key={item.to} item={item} t={t} />
      ))}
      <BottomNavLink item={MORE_ENTRY} t={t} />
    </nav>
  )
}

function BottomNavLink({
  item,
  t,
}: {
  item: MobileNavEntry
  t: (k: string) => string
}) {
  const Icon = item.icon
  return (
    <Link
      to={item.to}
      activeOptions={{ exact: item.exact }}
      className="flex flex-col items-center gap-1 text-on-surface-variant [&.active]:font-bold [&.active]:text-primary"
    >
      <Icon className="size-5" aria-hidden />
      <span className="text-xs font-bold tracking-wide uppercase">
        {t(item.labelKey)}
      </span>
    </Link>
  )
}
