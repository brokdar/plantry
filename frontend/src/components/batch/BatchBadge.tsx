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
  const { t, i18n } = useTranslation()
  const { highlight } = useBatchHighlight()
  const dashed = content.variant === "surplus" || content.variant === "toFreeze"
  // "2/2" alone is cryptic and read as "2 slash 2": say what it is.
  const spoken =
    content.variant === "leftover" || content.variant === "frozen"
      ? t(
          content.variant === "frozen"
            ? "plan.cell.speech_frozen"
            : "plan.cell.speech_leftover",
          { position: content.position, meals: content.meals }
        )
      : null

  return (
    <span
      data-batch-badge=""
      data-variant={content.variant}
      data-colour={colour}
      title={spoken ?? undefined}
      onPointerEnter={() => highlight(sessionId, colour)}
      className={cn(
        "inline-flex items-center gap-1 rounded-tag border px-2 py-0.5 text-caption font-semibold whitespace-nowrap tabular-nums",
        dashed
          ? cn("border-dashed", BATCH_CLASSES[colour].outline)
          : cn("border-transparent", BATCH_CLASSES[colour].solid),
        className
      )}
    >
      {spoken && <span className="sr-only">{spoken}</span>}
      <span
        data-batch-badge-text=""
        aria-hidden={spoken ? true : undefined}
        className="inline-flex items-center gap-1"
      >
        {content.variant === "frozen" && (
          <Snowflake className="size-3" strokeWidth={1.5} aria-hidden="true" />
        )}
        <BadgeText content={content} text={batchBadgeText(content, i18n)} />
        {suffix && ` · ${suffix}`}
      </span>
    </span>
  )
}

/** The cook strip's recipe name truncates (full name in `title`) so a long
 *  name can't stretch the strip, while the weekday and portions stay. */
function BadgeText({
  content,
  text,
}: {
  content: BatchBadgeContent
  text: string
}) {
  const name = content.variant === "cook" ? content.strip?.name : undefined
  const at = name ? text.indexOf(name) : -1
  if (!name || at < 0) return text
  return (
    <>
      {text.slice(0, at)}
      <span title={name} className="max-w-48 min-w-0 truncate">
        {name}
      </span>
      {text.slice(at + name.length)}
    </>
  )
}
