import { useTranslation } from "react-i18next"

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

interface ShortcutCheatsheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

interface Row {
  keys: string[]
  labelKey: string
}

const NAV_ROWS: Row[] = [
  { keys: ["←", "→", "↑", "↓"], labelKey: "planner.shortcuts.move_focus" },
  { keys: ["Home", "End"], labelKey: "planner.shortcuts.row_edge" },
  { keys: ["Enter"], labelKey: "planner.shortcuts.open_or_add" },
]

const ACTION_ROWS: Row[] = [
  { keys: ["S"], labelKey: "planner.shortcuts.skip" },
  { keys: ["Del"], labelKey: "planner.shortcuts.delete" },
  { keys: ["⌘", "Drag"], labelKey: "planner.shortcuts.copy_drag" },
  { keys: ["?"], labelKey: "planner.shortcuts.this_panel" },
]

export function ShortcutCheatsheet({
  open,
  onOpenChange,
}: ShortcutCheatsheetProps) {
  const { t } = useTranslation()
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-md"
        data-testid="shortcut-cheatsheet"
        aria-describedby={undefined}
      >
        <DialogHeader>
          <DialogTitle>{t("planner.shortcuts.title")}</DialogTitle>
          <DialogDescription className="sr-only">
            {t("planner.shortcuts.description")}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-5">
          <Section
            title={t("planner.shortcuts.nav_section")}
            rows={NAV_ROWS}
            t={t}
          />
          <Section
            title={t("planner.shortcuts.actions_section")}
            rows={ACTION_ROWS}
            t={t}
          />
        </div>
      </DialogContent>
    </Dialog>
  )
}

function Section({
  title,
  rows,
  t,
}: {
  title: string
  rows: Row[]
  t: (k: string) => string
}) {
  return (
    <section className="space-y-2">
      <h3 className="font-heading text-[10px] font-bold tracking-[0.18em] text-on-surface-variant uppercase">
        {title}
      </h3>
      <ul className="space-y-1.5">
        {rows.map((row) => (
          <li
            key={row.labelKey}
            className="flex items-center justify-between gap-3"
          >
            <span className="text-sm text-on-surface">{t(row.labelKey)}</span>
            <span className="flex shrink-0 items-center gap-1">
              {row.keys.map((k, i) => (
                <kbd
                  key={i}
                  className="grid min-w-[1.75rem] place-items-center rounded-md border border-outline-variant/60 bg-surface-container-lowest px-1.5 py-0.5 font-heading text-[11px] font-semibold text-on-surface shadow-sm"
                >
                  {k}
                </kbd>
              ))}
            </span>
          </li>
        ))}
      </ul>
    </section>
  )
}
