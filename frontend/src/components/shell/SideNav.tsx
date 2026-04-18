import { Link } from "@tanstack/react-router"
import {
  BookOpen,
  Bookmark,
  CalendarDays,
  Download,
  History,
  Leaf,
  Package,
  Settings,
} from "lucide-react"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"
import { GeneratePlanButton } from "./GeneratePlanButton"
import { SideNavItem } from "./SideNavItem"

export type SideNavVariant = "default" | "rail"

import type { LucideIcon } from "lucide-react"

type NavItem = {
  to: string
  labelKey: string
  icon: LucideIcon
  exact?: boolean
}

const NAV_ITEMS: NavItem[] = [
  { to: "/", labelKey: "nav.planner", icon: CalendarDays, exact: true },
  { to: "/components", labelKey: "nav.components", icon: BookOpen },
  { to: "/ingredients", labelKey: "nav.ingredients", icon: Package },
  { to: "/templates", labelKey: "nav.templates", icon: Bookmark },
  { to: "/import", labelKey: "nav.import", icon: Download },
  { to: "/archive", labelKey: "nav.archive", icon: History },
  { to: "/settings", labelKey: "nav.settings", icon: Settings },
]

type SideNavProps = {
  variant?: SideNavVariant
}

export function SideNav({ variant = "default" }: SideNavProps) {
  const { t } = useTranslation()

  if (variant === "rail") {
    return (
      <aside
        className={cn(
          "hide-scrollbar hidden h-screen w-20 shrink-0 flex-col overflow-y-auto",
          "sticky top-0 left-0 border-r border-outline-variant/10 bg-surface-container-low",
          "px-3 pt-8 pb-10 lg:flex"
        )}
        data-testid="sidenav-rail"
      >
        <div className="mb-8 flex justify-center">
          <Link
            to="/"
            aria-label={t("nav.brand")}
            className="gradient-primary flex size-10 items-center justify-center rounded-lg text-on-primary"
          >
            <Leaf className="size-5" aria-hidden />
          </Link>
        </div>
        <nav className="flex-1 space-y-3">
          {NAV_ITEMS.map((item) => (
            <SideNavItem
              key={item.to}
              to={item.to}
              label={t(item.labelKey)}
              icon={item.icon}
              exact={item.exact}
              variant="rail"
            />
          ))}
        </nav>
        <div className="mt-auto flex flex-col items-center gap-4">
          <GeneratePlanButton variant="rail" />
        </div>
      </aside>
    )
  }

  return (
    <aside
      className={cn(
        "sticky top-0 left-0 hidden h-screen w-64 shrink-0 flex-col",
        "bg-surface-container-low py-8 pr-0 pl-4 md:flex"
      )}
      data-testid="sidenav"
    >
      <Link
        to="/"
        aria-label={t("nav.brand")}
        className="mb-10 block px-4"
        data-testid="sidenav-brand"
      >
        <h1 className="font-heading text-xl font-bold tracking-tight text-on-surface">
          {t("nav.brand")}
        </h1>
        <p className="mt-1 text-xs font-medium tracking-widest text-on-surface-variant uppercase">
          {t("nav.atelier_tagline")}
        </p>
      </Link>
      <nav className="flex flex-1 flex-col gap-1 pr-4">
        {NAV_ITEMS.map((item) => (
          <SideNavItem
            key={item.to}
            to={item.to}
            label={t(item.labelKey)}
            icon={item.icon}
            exact={item.exact}
          />
        ))}
      </nav>
      <div className="mt-auto pr-4">
        <GeneratePlanButton />
      </div>
    </aside>
  )
}
