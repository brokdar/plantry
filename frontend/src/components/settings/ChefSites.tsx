import { X } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import type { ChefSite } from "@/lib/api/chef"
import { useChefSiteActions, useChefSites } from "@/lib/queries/chef"

import { Switch } from "./controls"

const LANGS = ["en", "de"] as const

/** The recipe sites finds search (FIND-7), grouped by language: switch a
 *  site on or off, add one, remove one you added, and try a site again that
 *  the search provider refuses for a while. */
export function ChefSites() {
  const { t } = useTranslation()
  const { data } = useChefSites()
  const actions = useChefSiteActions()
  if (!data) return null
  return (
    <div className="flex flex-col gap-6 py-5" data-testid="chef-sites">
      <p className="text-small text-ink-muted">
        {t("settings_page.assistant.sites_effect")}
      </p>
      {LANGS.map((lang) => {
        const sites = data.filter((s) => s.lang === lang)
        if (sites.length === 0) return null
        return (
          <section key={lang} className="flex flex-col gap-1">
            <h4 className="text-body font-semibold text-ink">
              {t(`settings_page.assistant.sites_lang_${lang}`)}
            </h4>
            <ul className="flex flex-col" data-testid={`chef-sites-${lang}`}>
              {sites.map((site) => (
                <SiteRow
                  key={site.domain}
                  site={site}
                  onToggle={(enabled) =>
                    actions.toggle.mutate({ domain: site.domain, enabled })
                  }
                  onRemove={() => actions.remove.mutate(site.domain)}
                  onRetry={() => actions.retry.mutate(site.domain)}
                />
              ))}
            </ul>
          </section>
        )
      })}
      <AddSite
        pending={actions.add.isPending}
        onAdd={(domain, lang) => actions.add.mutateAsync({ domain, lang })}
      />
    </div>
  )
}

function SiteRow({
  site,
  onToggle,
  onRemove,
  onRetry,
}: {
  site: ChefSite
  onToggle: (enabled: boolean) => void
  onRemove: () => void
  onRetry: () => void
}) {
  const { t, i18n } = useTranslation()
  const until = site.blocked_until
    ? new Date(site.blocked_until).toLocaleDateString(i18n.language, {
        day: "numeric",
        month: "long",
      })
    : null
  return (
    <li
      className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-line py-2 last:border-b-0"
      data-testid={`site-${site.domain}`}
    >
      <Switch
        checked={site.enabled}
        onChange={onToggle}
        label={t("settings_page.assistant.sites_toggle", {
          domain: site.domain,
        })}
        testId={`site-toggle-${site.domain}`}
      />
      <span className="text-body text-ink">{site.domain}</span>
      {until && (
        <span className="flex items-center gap-2 text-small text-ink-muted">
          {t("settings_page.assistant.sites_blocked", { date: until })}
          <Button
            type="button"
            variant="outline"
            className="h-9 pointer-coarse:h-11"
            onClick={onRetry}
            data-testid={`site-retry-${site.domain}`}
          >
            {t("settings_page.assistant.sites_retry")}
          </Button>
        </span>
      )}
      {!site.default && (
        <Button
          type="button"
          variant="ghost"
          className="ml-auto size-9 pointer-coarse:size-11"
          aria-label={t("settings_page.assistant.sites_remove", {
            domain: site.domain,
          })}
          onClick={onRemove}
          data-testid={`site-remove-${site.domain}`}
        >
          <X strokeWidth={1.5} aria-hidden />
        </Button>
      )}
    </li>
  )
}

function AddSite({
  pending,
  onAdd,
}: {
  pending: boolean
  onAdd: (domain: string, lang: "en" | "de") => Promise<unknown>
}) {
  const { t } = useTranslation()
  const [domain, setDomain] = useState("")
  const [lang, setLang] = useState<"en" | "de">("en")

  async function add() {
    if (domain.trim() === "") return
    try {
      await onAdd(domain, lang)
      setDomain("")
    } catch {
      // The mutation already toasted the reason; keep the typed domain.
    }
  }

  return (
    <div className="flex flex-wrap items-end gap-2">
      <div className="flex flex-col gap-1">
        <label htmlFor="site-domain" className="text-small text-ink-muted">
          {t("settings_page.assistant.sites_add_label")}
        </label>
        <Input
          id="site-domain"
          value={domain}
          maxLength={100}
          placeholder={t("settings_page.assistant.sites_placeholder")}
          onChange={(e) => setDomain(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault()
              void add()
            }
          }}
          className="w-64 pointer-coarse:h-11"
          data-testid="site-domain-input"
        />
      </div>
      <select
        aria-label={t("settings_page.assistant.sites_lang_label")}
        value={lang}
        onChange={(e) => setLang(e.target.value as "en" | "de")}
        className="h-9 rounded-md border border-line bg-panel px-2 text-body text-ink pointer-coarse:h-11"
        data-testid="site-lang-select"
      >
        {LANGS.map((l) => (
          <option key={l} value={l}>
            {t(`settings_page.assistant.sites_lang_${l}`)}
          </option>
        ))}
      </select>
      <Button
        type="button"
        variant="outline"
        disabled={pending || domain.trim() === ""}
        onClick={() => void add()}
        className="pointer-coarse:h-11"
        data-testid="site-add-button"
      >
        {t("settings_page.assistant.sites_add")}
      </Button>
    </div>
  )
}
