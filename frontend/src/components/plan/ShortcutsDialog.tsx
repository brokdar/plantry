import { useTranslation } from "react-i18next"

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

interface Row {
  keys: string[]
  labelKey: string
}

const ROWS: Row[] = [
  { keys: ["←", "→", "↑", "↓"], labelKey: "plan.shortcuts.move" },
  { keys: ["Home", "End"], labelKey: "plan.shortcuts.edges" },
  { keys: ["Enter"], labelKey: "plan.shortcuts.open" },
  { keys: ["N"], labelKey: "plan.shortcuts.no_plate" },
  { keys: ["Ctrl / ⌘", "drag"], labelKey: "plan.shortcuts.copy_drag" },
  { keys: ["?"], labelKey: "plan.shortcuts.this_panel" },
]

/** The board's `?` overlay listing its keyboard shortcuts (UX §6). */
export function ShortcutsDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-md bg-panel text-ink"
        data-testid="plan-shortcuts"
      >
        <DialogHeader>
          <DialogTitle>{t("plan.shortcuts.title")}</DialogTitle>
          <DialogDescription className="text-ink-muted">
            {t("plan.shortcuts.description")}
          </DialogDescription>
        </DialogHeader>
        <dl className="flex flex-col divide-y divide-line">
          {ROWS.map((row) => (
            <div
              key={row.labelKey}
              className="flex items-center justify-between gap-3 py-2"
            >
              <dt className="text-small text-ink">{t(row.labelKey)}</dt>
              <dd className="flex shrink-0 items-center gap-1">
                {row.keys.map((k) => (
                  <kbd
                    key={k}
                    className="grid min-w-7 place-items-center rounded-cell border border-line bg-panel-sunken px-1.5 py-0.5 text-caption font-semibold text-ink"
                  >
                    {k === "drag" ? t("plan.shortcuts.drag") : k}
                  </kbd>
                ))}
              </dd>
            </div>
          ))}
        </dl>
      </DialogContent>
    </Dialog>
  )
}
