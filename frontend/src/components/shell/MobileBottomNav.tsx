import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"
import { GeneratePlanButton } from "./GeneratePlanButton"
import { usePrimaryNav, type PrimaryNavEntry } from "./primary-nav"

export function MobileBottomNav() {
  const { t } = useTranslation()
  const items = usePrimaryNav()
  const left = items.slice(0, 2)
  const right = items.slice(2)

  return (
    <nav
      aria-label={t("nav.primary")}
      className={cn(
        "fixed bottom-0 left-0 z-40 grid w-full grid-cols-[1fr_1fr_auto_1fr_1fr] items-center gap-1 md:hidden",
        "h-[var(--tab-bar-inset)] rounded-t-2xl border-t border-line bg-panel px-2 pb-[env(safe-area-inset-bottom)]"
      )}
      data-testid="mobile-bottom-nav"
    >
      {left.map((item) => (
        <BottomNavLink key={item.id} item={item} label={t(item.labelKey)} />
      ))}
      <div className="relative -top-5 px-1">
        <GeneratePlanButton variant="fab" />
      </div>
      {right.map((item) => (
        <BottomNavLink key={item.id} item={item} label={t(item.labelKey)} />
      ))}
    </nav>
  )
}

function BottomNavLink({
  item,
  label,
}: {
  item: PrimaryNavEntry
  label: string
}) {
  const Icon = item.icon
  return (
    <Link
      to={item.to}
      activeOptions={{ exact: true }}
      aria-current={item.active ? "page" : undefined}
      data-active={item.active}
      className={cn(
        "flex min-h-11 min-w-0 flex-col items-center justify-center gap-0.5 rounded-lg text-ink-muted",
        "focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none",
        "data-[active=true]:font-semibold data-[active=true]:text-accent"
      )}
    >
      <Icon className="size-5" strokeWidth={1.5} aria-hidden />
      <span className="max-w-full truncate text-xs">{label}</span>
    </Link>
  )
}
