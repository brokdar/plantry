import { Snowflake } from "lucide-react"
import { useTranslation } from "react-i18next"

import { BATCH_CLASSES, type BatchColour } from "@/lib/plan/batch-colours"
import { formatAmount, type Formatter } from "@/lib/plan/format"
import { cn } from "@/lib/utils"

import { useBatchHighlight } from "./batch-highlight"

type BadgeContent =
  /** The cook cell: "Cook × 6". With `strip`, the cook strip's
   *  "Sun · Chili × 6". */
  | {
      variant: "cook"
      portions: number
      strip?: { weekday: string; name: string }
    }
  /** A linked cell: "2/3"; `frozen` adds "· frozen" and a snowflake. */
  | { variant: "leftover" | "frozen"; position: number; meals: number }
  /** Cook strip extras, dashed: "2 spare", "2 to freeze". */
  | { variant: "surplus" | "toFreeze"; count: number }

export type BatchBadgeProps = BadgeContent & {
  sessionId: number
  colour: BatchColour
  className?: string
}

function badgeText(content: BadgeContent, f: Formatter): string {
  switch (content.variant) {
    case "cook": {
      const portions = formatAmount(content.portions, f)
      return content.strip
        ? f.t("batch.strip", { ...content.strip, portions })
        : f.t("batch.cook", { portions })
    }
    case "leftover":
    case "frozen":
      return f.t(
        content.variant === "frozen" ? "batch.frozen" : "batch.linked",
        {
          position: content.position,
          meals: content.meals,
        }
      )
    case "surplus":
    case "toFreeze":
      return f.t(
        content.variant === "surplus" ? "batch.surplus" : "batch.to_freeze",
        { count: formatAmount(content.count, f) }
      )
  }
}

/**
 * The batch badge (UX §4, D28): a tag in the batch colour that always says
 * where the food comes from. Pointing at it (hover or tap) highlights every
 * cell of the batch through `BatchHighlightProvider`.
 */
export function BatchBadge({
  sessionId,
  colour,
  className,
  ...content
}: BatchBadgeProps) {
  const { i18n } = useTranslation()
  const { highlight } = useBatchHighlight()
  const dashed = content.variant === "surplus" || content.variant === "toFreeze"

  return (
    <span
      data-batch-badge=""
      data-variant={content.variant}
      data-colour={colour}
      onPointerEnter={() => highlight(sessionId)}
      className={cn(
        "inline-flex items-center gap-1 rounded-tag border px-2 py-0.5 text-caption font-semibold whitespace-nowrap tabular-nums",
        dashed
          ? cn("border-dashed", BATCH_CLASSES[colour].outline)
          : cn("border-transparent", BATCH_CLASSES[colour].solid),
        className
      )}
    >
      {content.variant === "frozen" && (
        <Snowflake className="size-3" strokeWidth={1.5} aria-hidden="true" />
      )}
      {badgeText(content, i18n)}
    </span>
  )
}
