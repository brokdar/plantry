import { Bookmark } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Separator } from "@/components/ui/separator"
import { Skeleton } from "@/components/ui/skeleton"
import type { Template } from "@/lib/api/templates"
import { useTemplates } from "@/lib/queries/templates"

interface ApplyTemplateSectionProps {
  onPick: (template: Template) => void
}

export function ApplyTemplateSection({ onPick }: ApplyTemplateSectionProps) {
  const { t } = useTranslation()
  const { data: templates, isLoading } = useTemplates()

  if (!isLoading && (!templates || templates.length === 0)) return null

  return (
    <div className="space-y-3" data-testid="apply-template-section">
      <div className="flex items-center gap-2">
        <Bookmark className="size-3.5 text-accent-foreground" aria-hidden />
        <p className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
          {t("template.apply_from")}
        </p>
      </div>
      {isLoading ? (
        <div className="space-y-2">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </div>
      ) : (
        <ul className="space-y-1.5">
          {templates?.map((tpl) => (
            <li key={tpl.id}>
              <button
                type="button"
                onClick={() => onPick(tpl)}
                className="group flex w-full items-center justify-between gap-3 rounded-md border border-border bg-card px-3 py-2 text-left transition hover:border-primary/40 hover:bg-accent/20"
                data-testid={`apply-template-${tpl.id}`}
              >
                <span className="truncate text-sm font-medium">{tpl.name}</span>
                <Badge variant="secondary" className="shrink-0 text-xs">
                  {t("template.components_count", {
                    count: tpl.components.length,
                  })}
                </Badge>
              </button>
            </li>
          ))}
        </ul>
      )}
      <Separator className="opacity-60" />
    </div>
  )
}
