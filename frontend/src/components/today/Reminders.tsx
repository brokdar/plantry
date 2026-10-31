import { useTranslation } from "react-i18next"

import type { TodayReminderView } from "@/lib/plan/today"

/** "Freezer · Take Chili out of the freezer tonight" and the last-day note
 *  for a leftover (UX §5.2). Renders nothing without reminders. */
export function Reminders({ reminders }: { reminders: TodayReminderView[] }) {
  const { t } = useTranslation()
  if (reminders.length === 0) return null
  return (
    <section aria-labelledby="today-reminders" className="flex flex-col">
      <h2 id="today-reminders" className="sr-only">
        {t("today.reminder.title")}
      </h2>
      <dl className="flex flex-col border-y border-line">
        {reminders.map((r) => (
          <div
            key={r.key}
            className="grid grid-cols-[6rem_minmax(0,1fr)] items-baseline gap-3 py-2.5"
            data-testid={`today-reminder-${r.kind}`}
          >
            <dt className="text-small font-semibold text-ink">{r.label}</dt>
            <dd className="text-body text-ink">{r.text}</dd>
          </div>
        ))}
      </dl>
    </section>
  )
}
