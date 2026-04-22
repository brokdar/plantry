import { useEffect, useState } from "react"
import { Link } from "@tanstack/react-router"
import {
  BookOpen,
  CalendarDays,
  ChevronLeft,
  History,
  Leaf,
  Package,
  Settings,
} from "lucide-react"
import type { LucideIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"
import { GeneratePlanButton } from "./GeneratePlanButton"
import { SideNavItem } from "./SideNavItem"

type NavItem = {
  to: string
  labelKey: string
  icon: LucideIcon
  exact?: boolean
}

const NAV_ITEMS: NavItem[] = [
  { to: "/", labelKey: "nav.planner", icon: CalendarDays, exact: true },
  { to: "/components", labelKey: "nav.recipes", icon: BookOpen },
  { to: "/ingredients", labelKey: "nav.ingredients", icon: Package },
  { to: "/archive", labelKey: "nav.past_weeks", icon: History },
  { to: "/settings", labelKey: "nav.settings", icon: Settings },
]

function useIsLg() {
  const [isLg, setIsLg] = useState(
    () => window.matchMedia("(min-width: 1024px)").matches
  )
  useEffect(() => {
    const mq = window.matchMedia("(min-width: 1024px)")
    const handler = (e: MediaQueryListEvent) => setIsLg(e.matches)
    mq.addEventListener("change", handler)
    return () => mq.removeEventListener("change", handler)
  }, [])
  return isLg
}

type SideNavProps = {
  collapsed: boolean
  onToggle: () => void
}

export function SideNav({ collapsed, onToggle }: SideNavProps) {
  const { t } = useTranslation()
  const isLg = useIsLg()
  const isRail = !isLg || collapsed

  return (
    <aside
      className={cn(
        "hide-scrollbar hidden h-screen shrink-0 flex-col overflow-y-auto md:flex",
        "sticky top-0 left-0 border-r border-outline-variant/10 bg-surface-container-low",
        "transition-[width] duration-200",
        isRail ? "w-20 px-3 pt-8 pb-10" : "w-64 py-8 pr-0 pl-4"
      )}
      data-testid="sidenav"
      data-collapsed={isRail}
    >
      {isRail ? (
        <div className="mb-8 flex justify-center">
          <Link
            to="/"
            aria-label={t("nav.brand")}
            className="gradient-primary flex size-10 items-center justify-center rounded-lg text-on-primary"
          >
            <Leaf className="size-5" aria-hidden />
          </Link>
        </div>
      ) : (
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
      )}

      <nav
        className={cn(
          "flex-1",
          isRail ? "space-y-3" : "flex flex-col gap-1 pr-4"
        )}
      >
        {NAV_ITEMS.map((item) => (
          <SideNavItem
            key={item.to}
            to={item.to}
            label={t(item.labelKey)}
            icon={item.icon}
            exact={item.exact}
            variant={isRail ? "rail" : "default"}
          />
        ))}
      </nav>

      <div
        className={cn(
          "mt-auto flex flex-col items-center gap-3 pt-4",
          !isRail && "pr-4"
        )}
      >
        <GeneratePlanButton variant={isRail ? "rail" : "default"} />
        {isLg && (
          <button
            type="button"
            onClick={onToggle}
            aria-label={
              collapsed ? t("nav.expand_sidebar") : t("nav.collapse_sidebar")
            }
            className="flex size-8 items-center justify-center rounded-lg text-on-surface-variant/50 transition-colors hover:bg-surface-container-high hover:text-on-surface-variant"
          >
            <ChevronLeft
              className={cn(
                "size-4 transition-transform duration-200",
                collapsed && "rotate-180"
              )}
              aria-hidden
            />
          </button>
        )}
      </div>
    </aside>
  )
}
