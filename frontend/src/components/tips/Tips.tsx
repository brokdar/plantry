import { Lightbulb } from "lucide-react"
import { createContext, use, type ReactNode } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import type { PlateFit, Tip } from "@/lib/api/tips"
import { useApplyTip, useDismissTip, useTips } from "@/lib/queries/tips"
import { cn } from "@/lib/utils"

interface TipsContextValue {
  tips: Map<number, Tip>
  fit: Map<number, PlateFit>
}

const TipsContext = createContext<TipsContextValue | null>(null)

/** Loads the tips of the window that holds `date` for the cells below it.
 *  While the Chef is off the server returns none, so nothing shows. */
export function TipsProvider({
  date,
  children,
}: {
  date: string | undefined
  children: ReactNode
}) {
  const { data } = useTips(date)
  const value: TipsContextValue = {
    tips: new Map(
      (data?.tips ?? [])
        .filter((tip) => tip.state === "shown" && tip.plate_id != null)
        .map((tip) => [tip.plate_id!, tip])
    ),
    fit: new Map((data?.on ? data.fit : []).map((f) => [f.plate_id, f])),
  }
  return <TipsContext value={value}>{children}</TipsContext>
}

function useTipsContext(): TipsContextValue | null {
  return use(TipsContext)
}

/** The tip sentence: what to do, then why. */
function tipSentence(
  tip: Tip,
  t: (key: string, opts?: Record<string, unknown>) => string
): string {
  const what = t(`tips.sentence.${tip.kind}`, {
    food: tip.food_name,
    from: tip.from_name ?? "",
  })
  return `${what} ${t(`tips.because.${tip.dimension}`)}`
}

/** The tip mark of a plate (TIP-7): a small bulb whose popover holds the
 *  sentence, Apply and Not now. Nothing on a plate without a tip (TIP-1). */
export function TipMark({
  plateId,
  className,
}: {
  plateId: number
  className?: string
}) {
  const tip = useTipsContext()?.tips.get(plateId)
  if (!tip) return null
  return <TipPopover tip={tip} className={className} />
}

function TipPopover({ tip, className }: { tip: Tip; className?: string }) {
  const { t } = useTranslation()
  const apply = useApplyTip()
  const dismiss = useDismissTip()
  const busy = apply.isPending || dismiss.isPending
  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          type="button"
          data-testid="tip-mark"
          aria-label={t("tips.mark_label")}
          className={cn(
            "relative inline-flex size-6 items-center justify-center rounded-tag bg-accent text-on-accent shadow-card",
            // A 44 px hit area around the small mark (NFR-8).
            "after:absolute after:-inset-2.5 after:content-['']",
            className
          )}
        >
          <Lightbulb className="size-3.5" strokeWidth={2} aria-hidden="true" />
        </button>
      </PopoverTrigger>
      <PopoverContent
        className="flex w-72 flex-col gap-3"
        data-testid="tip-popover"
      >
        <p className="text-caption font-semibold text-ink-muted">
          {t("tips.title")}
        </p>
        <p className="text-body text-ink">{tipSentence(tip, t)}</p>
        <div className="flex justify-end gap-2">
          <Button
            variant="ghost"
            size="sm"
            disabled={busy}
            onClick={() => dismiss.mutate(tip.id)}
          >
            {t("tips.dismiss")}
          </Button>
          <Button
            size="sm"
            disabled={busy}
            onClick={() => apply.mutate(tip.id)}
            data-testid="tip-apply"
          >
            {t("tips.apply")}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}

/** The plate's fit in one phrase (plate detail), so a good plate can be
 *  judged too. Nothing while the Chef is off. */
export function FitPhrase({ plateId }: { plateId: number }) {
  const { t } = useTranslation()
  const fit = useTipsContext()?.fit.get(plateId)
  if (!fit) return null
  const text =
    fit.raised.length === 0
      ? t("tips.fit.good")
      : t("tips.fit.lacks", {
          what: fit.raised.map((d) => t(`tips.fit.dim.${d}`)).join(", "),
        })
  return (
    <p className="text-small text-ink-muted" data-testid="fit-phrase">
      {text}
    </p>
  )
}
