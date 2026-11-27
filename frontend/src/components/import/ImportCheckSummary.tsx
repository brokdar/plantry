import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import type { ImportCheck } from "@/lib/api/import"

/**
 * The import check's one line ("Checked: 4 changes, 1 to look at"), each
 * applied change with Undo, the flags and the finishing-touch suggestion
 * (IMP-5). A check that ran and found nothing, or none, shows nothing; a
 * finding never blocks Save.
 */
export function ImportCheckSummary({
  check,
  undone,
  onUndo,
  touchAdded,
  onAddTouch,
}: {
  check: ImportCheck | null
  undone: ReadonlySet<string>
  onUndo: (id: string) => void
  touchAdded: boolean
  onAddTouch: () => void
}) {
  const { t } = useTranslation()
  if (!check?.ran) return null
  const touch = check.finishing_touch
  const look = check.flags.length + (touch ? 1 : 0)
  if (check.changes.length === 0 && look === 0) return null

  const parts = [
    check.changes.length > 0
      ? t("import.check.changes", { count: check.changes.length })
      : "",
    look > 0 ? t("import.check.look", { count: look }) : "",
  ].filter(Boolean)

  return (
    <section
      className="flex flex-col gap-2 rounded-md border border-line bg-panel-sunken p-3"
      data-testid="import-check"
      aria-label={t("import.check.title")}
    >
      <p className="text-sm font-medium text-ink">
        {t("import.check.prefix", { parts: parts.join(", ") })}
      </p>
      {check.cap_warning && (
        <p className="text-small text-ink-muted">
          {t("import.check.cap_warning")}
        </p>
      )}
      <ul className="flex flex-col gap-2">
        {check.changes.map((c) => (
          <li
            key={c.id}
            className="flex items-start justify-between gap-3 text-sm"
            data-testid="import-check-change"
          >
            <span>
              <span className={undone.has(c.id) ? "text-ink-muted" : ""}>
                {t(`import.check.kind.${c.kind}`)}
              </span>
              {c.reason && (
                <span className="block text-small text-ink-muted">
                  {c.reason}
                </span>
              )}
            </span>
            {undone.has(c.id) ? (
              <span className="text-small text-ink-muted">
                {t("import.check.undone")}
              </span>
            ) : (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-11 md:pointer-fine:h-8"
                onClick={() => onUndo(c.id)}
              >
                {t("common.undo")}
              </Button>
            )}
          </li>
        ))}
        {check.flags.map((f, i) => (
          <li key={i} className="text-sm" data-testid="import-check-flag">
            {f.text}
          </li>
        ))}
        {touch && (
          <li className="flex items-start justify-between gap-3 text-sm">
            <span>
              <span className="block text-small text-ink-muted">
                {t("import.check.touch")}
              </span>
              {touch}
            </span>
            {!touchAdded && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-11 md:pointer-fine:h-8"
                onClick={onAddTouch}
              >
                {t("import.check.touch_add")}
              </Button>
            )}
          </li>
        )}
      </ul>
    </section>
  )
}
