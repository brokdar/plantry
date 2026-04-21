import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { ArrowRight, ChevronDown, LayoutTemplate, Link2 } from "lucide-react"
import type { LucideIcon } from "lucide-react"

import { cn } from "@/lib/utils"

type PathCardProps = {
  to: string
  icon: LucideIcon
  title: string
  description: string
  testId?: string
}

function PathCard({
  to,
  icon: Icon,
  title,
  description,
  testId,
}: PathCardProps) {
  return (
    <Link
      to={to}
      data-testid={testId}
      className={cn(
        "group relative flex flex-col gap-3 overflow-hidden rounded-2xl border border-outline-variant/40 bg-surface-container-lowest p-5 transition-all",
        "hover:-translate-y-0.5 hover:border-primary/40 hover:shadow-[0_10px_30px_-12px_rgba(74,101,77,0.25)]"
      )}
    >
      <div className="flex items-start justify-between">
        <span className="flex size-10 items-center justify-center rounded-full bg-primary-fixed text-on-primary-fixed transition-transform group-hover:scale-105">
          <Icon className="size-5" aria-hidden />
        </span>
        <ArrowRight
          className="size-4 text-on-surface-variant transition-transform group-hover:translate-x-0.5 group-hover:text-primary"
          aria-hidden
        />
      </div>
      <div className="space-y-1">
        <h3 className="font-heading text-base font-bold text-on-surface">
          {title}
        </h3>
        <p className="text-sm text-on-surface-variant">{description}</p>
      </div>
    </Link>
  )
}

export function NewComponentPathChooser() {
  const { t } = useTranslation()

  return (
    <section
      aria-label={t("component.new_path_chooser.aria_label")}
      className="space-y-4"
      data-testid="new-component-path-chooser"
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <PathCard
          to="/import"
          icon={Link2}
          title={t("component.new_path_chooser.import_title")}
          description={t("component.new_path_chooser.import_description")}
          testId="path-chooser-import"
        />
        <PathCard
          to="/templates"
          icon={LayoutTemplate}
          title={t("component.new_path_chooser.template_title")}
          description={t("component.new_path_chooser.template_description")}
          testId="path-chooser-templates"
        />
      </div>
      <p className="flex items-center justify-center gap-1.5 text-xs tracking-wide text-on-surface-variant uppercase">
        <span className="h-px w-12 bg-outline-variant/60" aria-hidden />
        {t("component.new_path_chooser.or_manual")}
        <ChevronDown className="size-3.5" aria-hidden />
        <span className="h-px w-12 bg-outline-variant/60" aria-hidden />
      </p>
    </section>
  )
}
