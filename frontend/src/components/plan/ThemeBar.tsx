import { Plus, Sparkles, X } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import type { ThemeKind } from "@/lib/api/themes"
import {
  themeLabel,
  useThemeActions,
  useWindowThemes,
} from "@/lib/queries/themes"
import { cn } from "@/lib/utils"

const MAX_THEMES = 2

const KINDS: { kind: ThemeKind; label: string; placeholder: string }[] = [
  {
    kind: "cuisine",
    label: "plan.themes.kind.cuisine",
    placeholder: "plan.themes.placeholder.cuisine",
  },
  {
    kind: "ingredient",
    label: "plan.themes.kind.ingredient",
    placeholder: "plan.themes.placeholder.ingredient",
  },
  {
    kind: "mood",
    label: "plan.themes.kind.mood",
    placeholder: "plan.themes.placeholder.mood",
  },
]

/**
 * The window's week themes beside the window header (THEME-2..5): up to
 * two chips with remove, an Add popover (kind and a word), the window
 * before's themes as one-tap "again?", and the assistant's one suggestion
 * with Accept and No thanks. Shown only with the Chef on and a provider.
 */
export function ThemeBar({
  start,
  enabled,
}: {
  start: string
  enabled: boolean
}) {
  const { t } = useTranslation()
  const query = useWindowThemes(start, enabled)
  const actions = useThemeActions(start)
  if (!enabled || !query.data) return null
  const { themes, again, suggestion } = query.data
  const full = themes.length >= MAX_THEMES
  const busy =
    actions.set.isPending ||
    actions.accept.isPending ||
    actions.decline.isPending

  return (
    <section
      aria-label={t("plan.themes.title")}
      data-testid="theme-bar"
      className="flex flex-wrap items-center gap-2 text-small"
    >
      <span className="font-medium text-ink">{t("plan.themes.title")}</span>
      {themes.map((th) => (
        <span
          key={th.id}
          className="inline-flex items-center gap-1 rounded-tag bg-panel-sunken py-0.5 pr-1 pl-2.5 text-ink"
        >
          {themeLabel(th.value)}
          {th.whole_week && (
            <span className="text-ink-muted">
              {t("plan.themes.whole_week")}
            </span>
          )}
          <button
            type="button"
            className="rounded-full p-0.5 text-ink-muted hover:bg-panel hover:text-ink focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none"
            aria-label={t("plan.themes.remove", {
              name: themeLabel(th.value),
            })}
            disabled={actions.remove.isPending}
            onClick={() => actions.remove.mutate(th.id)}
          >
            <X className="size-3.5" aria-hidden />
          </button>
        </span>
      ))}
      {!full && (
        <AddTheme
          busy={busy}
          onAdd={(kind, value) => actions.set.mutateAsync({ kind, value })}
        />
      )}
      {themes.length === 0 && again.length > 0 && (
        <span className="inline-flex flex-wrap items-center gap-1 text-ink-muted">
          {t("plan.themes.last_week", {
            names: again.map((a) => themeLabel(a.value)).join(", "),
          })}
          <Button
            variant="ghost"
            size="sm"
            disabled={busy}
            onClick={async () => {
              for (const a of again) {
                await actions.set.mutateAsync({
                  kind: a.kind,
                  value: a.value,
                  source: "again",
                })
              }
            }}
          >
            {t("plan.themes.again")}
          </Button>
        </span>
      )}
      {themes.length === 0 && suggestion && (
        <span className="inline-flex flex-wrap items-center gap-1 text-ink-muted">
          <Sparkles className="size-3.5" aria-hidden />
          {t("plan.themes.suggestion", { name: themeLabel(suggestion.value) })}
          <Button
            variant="ghost"
            size="sm"
            disabled={busy}
            onClick={() => actions.accept.mutate()}
          >
            {t("plan.themes.accept")}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={busy}
            onClick={() => actions.decline.mutate()}
          >
            {t("plan.themes.decline")}
          </Button>
        </span>
      )}
    </section>
  )
}

function AddTheme({
  busy,
  onAdd,
}: {
  busy: boolean
  onAdd: (kind: ThemeKind, value: string) => Promise<unknown>
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [kind, setKind] = useState<ThemeKind>("cuisine")
  const [value, setValue] = useState("")
  const current = KINDS.find((k) => k.kind === kind) ?? KINDS[0]

  const submit = async () => {
    const v = value.trim()
    if (!v) return
    try {
      await onAdd(kind, v)
      setValue("")
      setOpen(false)
    } catch {
      // The mutation's toast says what went wrong; the popover stays open.
    }
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="ghost" size="sm" className="gap-1">
          <Plus className="size-3.5" aria-hidden />
          {t("plan.themes.add")}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="flex w-72 flex-col gap-3">
        <div
          role="radiogroup"
          aria-label={t("plan.themes.kind_label")}
          className="flex flex-wrap gap-1"
        >
          {KINDS.map((k) => (
            <button
              key={k.kind}
              type="button"
              role="radio"
              aria-checked={kind === k.kind}
              onClick={() => setKind(k.kind)}
              className={cn(
                "rounded-tag px-2.5 py-1 text-small focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none",
                kind === k.kind
                  ? "bg-accent text-on-accent"
                  : "bg-panel-sunken text-ink hover:bg-panel"
              )}
            >
              {t(k.label)}
            </button>
          ))}
        </div>
        <form
          className="flex gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            void submit()
          }}
        >
          <Input
            aria-label={t("plan.themes.value_label")}
            placeholder={t(current.placeholder)}
            value={value}
            maxLength={40}
            onChange={(e) => setValue(e.target.value)}
          />
          <Button type="submit" disabled={busy || !value.trim()}>
            {t("plan.themes.add_submit")}
          </Button>
        </form>
      </PopoverContent>
    </Popover>
  )
}
