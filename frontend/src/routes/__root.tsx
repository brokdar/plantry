import { createRootRoute, Link, Outlet } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { AppShell } from "@/components/shell/AppShell"
import { Button } from "@/components/ui/button"

export const Route = createRootRoute({
  component: RootComponent,
  notFoundComponent: NotFoundPage,
  errorComponent: ErrorPage,
})

function RootComponent() {
  return (
    <AppShell>
      <Outlet />
    </AppShell>
  )
}

function NotFoundPage() {
  const { t } = useTranslation()
  return (
    <div className="mx-auto flex max-w-lg flex-col items-center gap-4 px-4 py-24 text-center">
      <p className="font-heading text-7xl font-bold text-primary opacity-20">
        404
      </p>
      <h1 className="font-heading text-2xl font-bold text-on-surface">
        {t("error.page_not_found")}
      </h1>
      <p className="text-sm text-on-surface-variant">
        {t("error.page_not_found_body")}
      </p>
      <Button asChild>
        <Link to="/">{t("nav.planner")}</Link>
      </Button>
    </div>
  )
}

function ErrorPage() {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-dvh flex-col items-center justify-center gap-4 px-4 text-center">
      <h1 className="font-heading text-2xl font-bold text-on-surface">
        {t("error.server")}
      </h1>
      <p className="text-sm text-on-surface-variant">
        {t("error.page_error_body")}
      </p>
      <Button onClick={() => window.location.reload()}>
        {t("error.reload")}
      </Button>
    </div>
  )
}
