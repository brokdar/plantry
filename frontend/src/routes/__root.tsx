import {
  createRootRoute,
  Link,
  Outlet,
  useRouterState,
} from "@tanstack/react-router"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { CommandPalette } from "@/components/command/CommandPalette"
import { CopyFromWeekDialog } from "@/components/plan/CopyFromWeekDialog"
import { AppShell } from "@/components/shell/AppShell"
import { Button } from "@/components/ui/button"
import { Toaster } from "@/components/ui/sonner"
import { useCommandPaletteHotkey } from "@/lib/use-command-palette-hotkey"
import { initUnitsVocabulary } from "@/lib/domain/units"
import { useUnits } from "@/lib/queries/units"

export const Route = createRootRoute({
  component: RootComponent,
  notFoundComponent: NotFoundPage,
  errorComponent: ErrorPage,
})

/** Routes rendered without the app shell: `/setup`, `/cook/$sessionId` and
 *  `/recipes/$id/cook`. */
const BARE_ROUTE = /^\/(setup|cook)(\/|$)|^\/recipes\/[^/]+\/cook\/?$/

function RootComponent() {
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [copyWeekOpen, setCopyWeekOpen] = useState(false)
  useCommandPaletteHotkey(() => setPaletteOpen((prev) => !prev))

  const { data: units } = useUnits()
  useEffect(() => {
    if (units) initUnitsVocabulary(units)
  }, [units])

  // The setup wizard is a centred single column without the app shell
  // (UX §5.1), so a new install isn't offered navigation it can't use yet.
  // Cook mode is full screen (UX §5.3).
  const isBare = useRouterState({
    select: (s) => BARE_ROUTE.test(s.location.pathname),
  })
  if (isBare) {
    return (
      <>
        <Outlet />
        <Toaster />
      </>
    )
  }

  return (
    <AppShell>
      <Outlet />
      <CommandPaletteHost
        paletteOpen={paletteOpen}
        setPaletteOpen={setPaletteOpen}
        copyWeekOpen={copyWeekOpen}
        setCopyWeekOpen={setCopyWeekOpen}
      />
    </AppShell>
  )
}

/** Hosts CommandPalette + CopyFromWeekDialog. Computes "today" and
 *  "seven days ago" via Date.now(), which the React compiler flags as impure
 *  when called during render. Isolating it here keeps the rest of the root
 *  component compiler-friendly. The Date.now() call is invoked once per
 *  render but only used to seed dialog state. */
function CommandPaletteHost({
  paletteOpen,
  setPaletteOpen,
  copyWeekOpen,
  setCopyWeekOpen,
}: {
  paletteOpen: boolean
  setPaletteOpen: (next: boolean) => void
  copyWeekOpen: boolean
  setCopyWeekOpen: (next: boolean) => void
}) {
  const [today] = useState(() => new Date().toISOString().slice(0, 10))
  const [sevenDaysAgo] = useState(() =>
    new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString().slice(0, 10)
  )
  return (
    <>
      <CommandPalette
        open={paletteOpen}
        onOpenChange={setPaletteOpen}
        onOpenCopyWeek={() => setCopyWeekOpen(true)}
        defaultTargetDate={today}
      />
      <CopyFromWeekDialog
        open={copyWeekOpen}
        onOpenChange={setCopyWeekOpen}
        targetStart={today}
        defaultSourceStart={sevenDaysAgo}
      />
    </>
  )
}

/** Unknown paths, including the removed `/calendar`, `/day/$date` and
 *  `/archive` (plan D2: no redirects). */
function NotFoundPage() {
  const { t } = useTranslation()
  return (
    <div
      className="mx-auto flex max-w-lg flex-col items-start gap-3 px-4 py-16 md:px-8"
      data-testid="not-found"
    >
      <h1 className="text-h1 text-ink">{t("error.page_not_found")}</h1>
      <p className="text-body text-ink-muted">
        {t("error.page_not_found_body")}
      </p>
      <Button asChild className="h-11 px-4 md:pointer-fine:h-9">
        <Link to="/today">{t("error.go_today")}</Link>
      </Button>
    </div>
  )
}

function ErrorPage() {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-dvh flex-col items-center justify-center gap-4 px-4 text-center">
      <h1 className="text-h1 text-ink">{t("error.server")}</h1>
      <p className="text-body text-ink-muted">{t("error.page_error_body")}</p>
      <div className="flex flex-wrap justify-center gap-2">
        <Button
          className="h-11 px-4 md:pointer-fine:h-9"
          onClick={() => window.location.reload()}
        >
          {t("error.reload")}
        </Button>
        <Button
          asChild
          variant="outline"
          className="h-11 px-4 md:pointer-fine:h-9"
        >
          <Link to="/today">{t("error.go_today")}</Link>
        </Button>
      </div>
    </div>
  )
}
