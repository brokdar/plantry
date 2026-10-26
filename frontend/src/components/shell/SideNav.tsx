import { useEffect, useState } from "react"
import { Link, useLocation } from "@tanstack/react-router"
import { ChevronLeft, Leaf, Settings } from "lucide-react"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"
import { GeneratePlanButton } from "./GeneratePlanButton"
import { usePrimaryNav } from "./primary-nav"
import { SideNavItem } from "./SideNavItem"

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
  const items = usePrimaryNav()
  const settingsActive = useLocation({
    select: (l) => l.pathname.startsWith("/settings"),
  })

  return (
    <aside
      className={cn(
        "hide-scrollbar hidden h-screen shrink-0 flex-col overflow-y-auto md:flex",
        "sticky top-0 left-0 border-r border-line bg-panel-sunken",
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
          <h1 className="font-heading text-xl font-bold tracking-tight text-ink">
            {t("nav.brand")}
          </h1>
          <p className="mt-1 text-xs font-medium text-ink-muted">
            {t("nav.atelier_tagline")}
          </p>
        </Link>
      )}

      <nav
        aria-label={t("nav.primary")}
        className={cn("flex flex-1 flex-col", isRail ? "gap-2" : "gap-1 pr-4")}
      >
        {items.map((item) => (
          <SideNavItem
            key={item.id}
            to={item.to}
            label={t(item.labelKey)}
            icon={item.icon}
            active={item.active}
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
        {isRail ? (
          <Link
            to="/settings"
            aria-label={t("nav.settings")}
            title={t("nav.settings")}
            aria-current={settingsActive ? "page" : undefined}
            data-active={settingsActive}
            className="flex size-10 items-center justify-center rounded-lg text-ink-muted transition-colors duration-150 ease-out hover:bg-panel hover:text-ink focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none data-[active=true]:bg-accent/10 data-[active=true]:text-accent"
            data-testid="sidenav-settings"
          >
            <Settings className="size-5" strokeWidth={1.5} aria-hidden />
          </Link>
        ) : (
          <div className="w-full">
            <SideNavItem
              to="/settings"
              label={t("nav.settings")}
              icon={Settings}
              active={settingsActive}
            />
          </div>
        )}
        {isLg && (
          <button
            type="button"
            onClick={onToggle}
            aria-label={
              collapsed ? t("nav.expand_sidebar") : t("nav.collapse_sidebar")
            }
            className="flex size-8 items-center justify-center rounded-lg text-ink-muted transition-colors hover:bg-panel hover:text-ink"
          >
            <ChevronLeft
              className={cn(
                "size-4 transition-transform duration-200",
                collapsed && "rotate-180"
              )}
              strokeWidth={1.5}
              aria-hidden
            />
          </button>
        )}
      </div>
    </aside>
  )
}
