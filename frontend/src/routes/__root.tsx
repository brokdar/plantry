import { createRootRoute, Link, Outlet } from "@tanstack/react-router"
import { Leaf } from "lucide-react"
import { useTranslation } from "react-i18next"

export const Route = createRootRoute({
  component: RootComponent,
})

function RootComponent() {
  const { t } = useTranslation()

  return (
    <div className="min-h-svh bg-background text-foreground">
      <header className="border-b border-border">
        <nav className="mx-auto flex max-w-6xl items-center gap-3 px-6 py-4">
          <Link to="/" className="flex items-center gap-2">
            <Leaf className="h-5 w-5 text-primary" aria-hidden="true" />
            <span className="text-lg font-semibold">{t("nav.brand")}</span>
          </Link>
          <Link
            to="/ingredients"
            className="text-sm font-medium text-muted-foreground transition-colors hover:text-foreground [&.active]:text-foreground"
          >
            {t("nav.ingredients")}
          </Link>
          <Link
            to="/components"
            className="text-sm font-medium text-muted-foreground transition-colors hover:text-foreground [&.active]:text-foreground"
          >
            {t("nav.components")}
          </Link>
        </nav>
      </header>
      <main className="mx-auto max-w-6xl px-6 py-8">
        <Outlet />
      </main>
    </div>
  )
}
