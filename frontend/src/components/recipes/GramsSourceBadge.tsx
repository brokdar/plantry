import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import type { GramsSource } from "@/lib/domain/units"

type BadgeVariant = "secondary" | "outline" | "destructive"

const VARIANT: Record<GramsSource, BadgeVariant> = {
  direct: "secondary",
  portion: "secondary",
  default: "secondary",
  fallback: "outline",
  manual: "outline",
  unresolved: "destructive",
}

const LABEL_KEY: Record<GramsSource, string> = {
  direct: "component.grams_source.exact",
  portion: "component.grams_source.exact",
  default: "component.grams_source.exact",
  fallback: "component.grams_source.approx",
  manual: "component.grams_source.manual",
  unresolved: "component.grams_source.required",
}

const TOOLTIP_KEY: Record<GramsSource, string> = {
  direct: "component.grams_source.tooltip.direct",
  portion: "component.grams_source.tooltip.portion",
  default: "component.grams_source.tooltip.default",
  fallback: "component.grams_source.tooltip.fallback",
  manual: "component.grams_source.tooltip.manual",
  unresolved: "component.grams_source.tooltip.unresolved",
}

type GramsSourceBadgeProps = {
  source: GramsSource
  testId?: string
}

export function GramsSourceBadge({ source, testId }: GramsSourceBadgeProps) {
  const { t } = useTranslation()
  return (
    <TooltipProvider delayDuration={150}>
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge
            variant={VARIANT[source]}
            className="cursor-help text-[10px] tracking-wide uppercase"
            data-testid={testId}
            data-source={source}
            tabIndex={0}
          >
            {t(LABEL_KEY[source])}
          </Badge>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-[240px]">
          {t(TOOLTIP_KEY[source])}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
