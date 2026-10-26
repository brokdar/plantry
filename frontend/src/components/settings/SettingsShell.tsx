import { Link } from "@tanstack/react-router"
import {
  CalendarDays,
  ChevronLeft,
  ChevronRight,
  CookingPot,
  Database,
  MessageSquareText,
  Users,
  type LucideIcon,
} from "lucide-react"
import { useTranslation } from "react-i18next"

import { TooltipProvider } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"

import { AssistantSection } from "./sections/AssistantSection"
import { CookingSection } from "./sections/CookingSection"
import { HouseholdSection } from "./sections/HouseholdSection"
import { MealsSection } from "./sections/MealsSection"
import { SystemSection } from "./sections/SystemSection"
import {
  DEFAULT_SETTINGS_SECTION,
  SETTINGS_SECTIONS,
  type SettingsSection,
} from "./settings-sections"

const ICONS: Record<SettingsSection, LucideIcon> = {
  household: Users,
  meals: CalendarDays,
  cooking: CookingPot,
  assistant: MessageSquareText,
  system: Database,
}

const CONTENT: Record<SettingsSection, () => React.ReactNode> = {
  household: HouseholdSection,
  meals: MealsSection,
  cooking: CookingSection,
  assistant: AssistantSection,
  system: SystemSection,
}

/**
 * Settings: a section list beside the open section from 768 px up. On a
 * phone the list stands alone at `/settings` and drills into a section,
 * which shows a back link instead of the list.
 */
export function SettingsShell({ section }: { section?: SettingsSection }) {
  const { t } = useTranslation()
  // Wider screens always show a section; the bare `/settings` list only
  // exists on phones (the route redirects elsewhere).
  const shown = section ?? DEFAULT_SETTINGS_SECTION
  const Content = CONTENT[shown]

  return (
    <TooltipProvider delayDuration={200}>
      <div className="mx-auto flex w-full max-w-6xl flex-col gap-6 px-4 py-6 md:gap-8 md:px-8 md:py-10">
        <h1
          className={cn(
            "text-h1 font-semibold text-ink",
            section && "sr-only md:not-sr-only"
          )}
        >
          {t("settings_page.heading")}
        </h1>

        <div className="md:grid md:grid-cols-[12rem_minmax(0,1fr)] md:gap-8 lg:grid-cols-[16rem_minmax(0,1fr)] lg:gap-10">
          <nav
            aria-label={t("settings_page.nav_label")}
            className={cn("md:block", section ? "hidden" : "block")}
          >
            <ul className="flex flex-col divide-y divide-line md:gap-1 md:divide-y-0">
              {SETTINGS_SECTIONS.map((id) => (
                <SectionLink
                  key={id}
                  id={id}
                  current={id === section}
                  highlighted={id === shown}
                />
              ))}
            </ul>
          </nav>

          <section
            aria-labelledby="settings-section-title"
            className={cn(
              "min-w-0 flex-col gap-6 md:flex",
              section ? "flex" : "hidden"
            )}
            data-testid={`settings-section-${shown}`}
          >
            <div className="flex flex-col gap-2">
              <Link
                to="/settings"
                aria-label={t("settings_page.back_label")}
                className="-ml-2 inline-flex min-h-11 items-center gap-1 self-start rounded-lg px-2 text-body text-accent md:hidden"
              >
                <ChevronLeft className="size-5" strokeWidth={1.5} aria-hidden />
                {t("settings_page.back")}
              </Link>
              <h2
                id="settings-section-title"
                className="text-h2 font-semibold text-ink"
              >
                {t(`settings_page.sections.${shown}.title`)}
              </h2>
              <p className="text-body text-ink-muted">
                {t(`settings_page.sections.${shown}.summary`)}
              </p>
            </div>
            <Content />
          </section>
        </div>
      </div>
    </TooltipProvider>
  )
}

function SectionLink({
  id,
  current,
  highlighted,
}: {
  id: SettingsSection
  /** The open section's page. */
  current: boolean
  /** Marked in the side list from 768 px up. */
  highlighted: boolean
}) {
  const { t } = useTranslation()
  const Icon = ICONS[id]
  return (
    <li>
      <Link
        to="/settings/$section"
        params={{ section: id }}
        aria-current={current ? "page" : undefined}
        data-testid={`settings-nav-${id}`}
        className={cn(
          "flex min-h-14 items-center gap-3 py-3 text-ink transition-colors duration-150 ease-out md:min-h-11 md:rounded-lg md:px-3 md:py-2",
          highlighted
            ? "md:bg-accent/10 md:font-semibold md:text-accent"
            : "md:text-ink-muted md:hover:bg-panel md:hover:text-ink"
        )}
      >
        <Icon className="size-5 shrink-0" strokeWidth={1.5} aria-hidden />
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="text-body">
            {t(`settings_page.sections.${id}.title`)}
          </span>
          <span className="text-small text-ink-muted md:hidden">
            {t(`settings_page.sections.${id}.summary`)}
          </span>
        </span>
        <ChevronRight
          className="size-5 shrink-0 text-ink-muted md:hidden"
          strokeWidth={1.5}
          aria-hidden
        />
      </Link>
    </li>
  )
}
