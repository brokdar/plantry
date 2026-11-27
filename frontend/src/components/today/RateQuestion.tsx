import { useTranslation } from "react-i18next"

import type { NotForMeReason } from "@/lib/api/foods"
import { useNotForMe } from "@/lib/queries/foods"

import type { PlateFeedbackStatus } from "@/lib/api/feedback"
import { focusPageStart } from "@/lib/focus"
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
  const { rate: answer, answered, archived, dismissArchived } = useRatePlate()
  if (archived) {
    return (
      <TrialReason
        name={archived.name}
        id={archived.food_id}
        onDone={dismissArchived}
      />
    )
  }
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
            onClick={() => {
              // The question hides once answered; keep focus on the page.
              focusPageStart()
              answer(rate.plateId, status)
            }}
            className="inline-flex min-h-11 items-center rounded-full border border-line px-4 text-small text-ink transition-colors duration-150 ease-out hover:bg-panel-sunken md:pointer-fine:min-h-9"
          >
            {t(`today.rate.${status}`)}
          </button>
        ))}
      </div>
    </section>
  )
}

const REASONS: NotForMeReason[] = [
  "taste",
  "ingredients",
  "too_long",
  "looks_wrong",
]

/** After a disliked first cooking the recipe is already archived (NEW-4):
 *  one tap on a reason feeds taste; Undo comes with the toast. */
function TrialReason({
  id,
  name,
  onDone,
}: {
  id: number
  name: string
  onDone: () => void
}) {
  const { t } = useTranslation()
  const notForMe = useNotForMe()
  return (
    <section
      aria-label={t("today.rate_reason.prompt", { name })}
      className="flex flex-col gap-3"
      data-testid="today-rate-reason"
    >
      <h2 className="font-sans text-body font-semibold text-ink">
        {t("today.rate_reason.prompt", { name })}
      </h2>
      <div className="flex flex-wrap gap-2">
        {REASONS.map((reason) => (
          <button
            key={reason}
            type="button"
            disabled={notForMe.isPending}
            onClick={() =>
              notForMe.mutate({ id, reason }, { onSuccess: onDone })
            }
            className="inline-flex min-h-11 items-center rounded-full border border-line px-4 text-small text-ink transition-colors duration-150 ease-out hover:bg-panel-sunken md:pointer-fine:min-h-9"
          >
            {t(`recipe_detail.not_for_me.${reason}`)}
          </button>
        ))}
        <button
          type="button"
          onClick={onDone}
          className="inline-flex min-h-11 items-center rounded-full px-4 text-small text-ink-muted hover:bg-panel-sunken md:pointer-fine:min-h-9"
        >
          {t("common.close")}
        </button>
      </div>
    </section>
  )
}
