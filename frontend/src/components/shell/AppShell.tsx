import { useMatches } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { Toaster } from "@/components/ui/sonner"

import { MobileBottomNav } from "./MobileBottomNav"
import { SideNav, type SideNavVariant } from "./SideNav"
import { TopBar } from "./TopBar"

type AppShellProps = {
  children: React.ReactNode
}

type ShellStaticData = { shellVariant?: SideNavVariant }

export function AppShell({ children }: AppShellProps) {
  const { t } = useTranslation()
  const matches = useMatches()
  const lastMatch = matches[matches.length - 1]
  const sidebarVariant: SideNavVariant =
    (lastMatch?.staticData as ShellStaticData | undefined)?.shellVariant ??
    "default"

  return (
    <div className="flex min-h-svh bg-surface text-on-surface">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:fixed focus:top-4 focus:left-4 focus:z-50 focus:rounded-lg focus:bg-primary focus:px-4 focus:py-2 focus:text-sm focus:font-bold focus:text-on-primary focus:shadow-lg focus:outline-none"
      >
        {t("common.skip_to_content")}
      </a>
      <SideNav variant={sidebarVariant} />
      <div className="flex min-w-0 flex-1 flex-col pb-24 md:pb-0">
        <TopBar />
        <main id="main" className="flex-1">
          {children}
        </main>
      </div>
      <MobileBottomNav />
      <Toaster />
    </div>
  )
}
