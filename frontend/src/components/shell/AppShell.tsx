import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { Toaster } from "@/components/ui/sonner"
import { useProfile } from "@/lib/queries/profile"

import { MobileBottomNav } from "./MobileBottomNav"
import { SideNav } from "./SideNav"
import { TopBar } from "./TopBar"

type AppShellProps = {
  children: React.ReactNode
}

export function AppShell({ children }: AppShellProps) {
  const { t, i18n } = useTranslation()
  const { data: profile } = useProfile()

  const [collapsed, setCollapsed] = useState(
    () => localStorage.getItem("sidenav-collapsed") === "true"
  )

  function handleToggle() {
    setCollapsed((prev) => {
      const next = !prev
      localStorage.setItem("sidenav-collapsed", String(next))
      return next
    })
  }

  // Apply the user's saved locale to i18n once the profile loads. Without
  // this the LanguageDetector fallback (browser language) wins even when the
  // user explicitly picked a different language in settings.
  useEffect(() => {
    const saved = profile?.locale
    if (!saved) return
    const current = i18n.language.split("-")[0]
    if (saved !== current) void i18n.changeLanguage(saved)
  }, [profile?.locale, i18n])

  return (
    <div className="flex min-h-svh bg-surface text-on-surface">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:fixed focus:top-4 focus:left-4 focus:z-50 focus:rounded-lg focus:bg-primary focus:px-4 focus:py-2 focus:text-sm focus:font-bold focus:text-on-primary focus:shadow-lg focus:outline-none"
      >
        {t("common.skip_to_content")}
      </a>
      <SideNav collapsed={collapsed} onToggle={handleToggle} />
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
