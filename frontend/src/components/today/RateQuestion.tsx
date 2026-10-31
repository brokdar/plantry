import { useTranslation } from "react-i18next"

import type { PlateFeedbackStatus } from "@/lib/api/feedback"
import type { TodayRateView } from "@/lib/plan/today"
import { useRatePlate } from "@/lib/queries/feedback"

const ANSWERS: PlateFeedbackStatus[] = ["loved", "disliked", "not_eaten"]

/**
 * One rating question at a time (UX §5.2): "How was Sunday's Chili?" with
 * Loved it, Not again and Didn't eat it. It hides as soon as it is
 * answered; the toast offers Undo.
 */
export function RateQuestion({ rate }: { rate: TodayRateView | null }) {
  const { t } = useTranslation()
  const { rate: answer, answered } = useRatePlate()
  if (!rate || answered === rate.plateId) return null
  return (
    <section
      aria-labelledby="today-rate"
      className="flex flex-col gap-3"
      data-testid="today-rate"
    >
      <h2
        id="today-rate"
        className="font-sans text-body font-semibold text-ink"
      >
        {rate.question}
      </h2>
      <div className="flex flex-wrap gap-2">
        {ANSWERS.map((status) => (
          <button
            key={status}
            type="button"
            onClick={() => answer(rate.plateId, status)}
            className="inline-flex min-h-11 items-center rounded-full border border-line px-4 text-small text-ink transition-colors duration-150 ease-out hover:bg-panel-sunken md:pointer-fine:min-h-9"
          >
            {t(`today.rate.${status}`)}
          </button>
        ))}
      </div>
    </section>
  )
}
