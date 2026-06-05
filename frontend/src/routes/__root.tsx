import { createRootRoute, Link, Outlet } from "@tanstack/react-router"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { CommandPalette } from "@/components/command/CommandPalette"
import { CopyFromWeekDialog } from "@/components/presets/CopyFromWeekDialog"
import { AppShell } from "@/components/shell/AppShell"
import { Button } from "@/components/ui/button"
import { useCommandPaletteHotkey } from "@/lib/use-command-palette-hotkey"
import { initUnitsVocabulary } from "@/lib/domain/units"
import { useUnits } from "@/lib/queries/units"

export const Route = createRootRoute({
  component: RootComponent,
  notFoundComponent: NotFoundPage,
  errorComponent: ErrorPage,
})

function RootComponent() {
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [copyWeekOpen, setCopyWeekOpen] = useState(false)
  useCommandPaletteHotkey(() => setPaletteOpen((prev) => !prev))

  const { data: units } = useUnits()
  useEffect(() => {
    if (units) initUnitsVocabulary(units)
  }, [units])
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
