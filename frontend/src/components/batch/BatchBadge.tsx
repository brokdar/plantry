import { Snowflake } from "lucide-react"
import { useTranslation } from "react-i18next"

import { BATCH_CLASSES, type BatchColour } from "@/lib/plan/batch-colours"
import { batchBadgeText, type BatchBadgeContent } from "@/lib/plan/format"
import { cn } from "@/lib/utils"

import { useBatchHighlight } from "./batch-highlight"

export type BatchBadgeProps = BatchBadgeContent & {
  sessionId: number
  colour: BatchColour
  /** Appended after a middle dot, e.g. Today's "use by Wed". */
  suffix?: string
  className?: string
}

/**
 * The batch badge (UX §4, D28): a tag in the batch colour that always says
 * where the food comes from. Pointing at it (hover or tap) highlights every
 * cell of the batch through `BatchHighlightProvider`. Solid variants put
 * `ground` text on the batch colour; `surplus` and `toFreeze` are dashed
 * outlines, and `frozen` adds a snowflake.
 */
export function BatchBadge({
  sessionId,
  colour,
  suffix,
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
      onPointerEnter={() => highlight(sessionId, colour)}
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
      {batchBadgeText(content, i18n)}
      {suffix && ` · ${suffix}`}
    </span>
  )
}
