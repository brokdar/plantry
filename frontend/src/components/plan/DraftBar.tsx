import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"

interface DraftBarProps {
  /** "Draft · 10 plates · 4 cook days". */
  summary: string
  disabled: boolean
  onAccept: () => void
}

/** Phones: the sticky bar above the tab bar with Accept and shop (UX §5.5);
 *  the toolbar carries the button from 768 px. */
export function DraftBar({ summary, disabled, onAccept }: DraftBarProps) {
  const { t } = useTranslation()
  return (
    <div
      data-testid="draft-bar"
      className="sticky bottom-[var(--tab-bar-inset)] z-10 -mx-4 flex items-center justify-between gap-3 border-t border-line bg-ground px-4 py-3 md:hidden"
    >
      <span className="min-w-0 text-small text-ink-muted tabular-nums">
        {summary}
      </span>
      <Button
        type="button"
        className="h-11 shrink-0"
        disabled={disabled}
        onClick={onAccept}
      >
        {t("plan.review.accept_and_shop")}
      </Button>
    </div>
  )
}
