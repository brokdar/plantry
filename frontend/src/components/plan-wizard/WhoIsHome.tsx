import { X } from "lucide-react"
import { useId } from "react"
import { useTranslation } from "react-i18next"

import {
  AvailabilityGrid,
  type AvailabilityGridColumn,
  type AvailabilityGridSlot,
  type AvailabilityGridValue,
} from "@/components/availability/AvailabilityGrid"
import type { RoutinePreset } from "@/components/availability/RoutinePicker"
import { QuantityStepper } from "@/components/plan/QuantityStepper"
import { Button } from "@/components/ui/button"
import { formatDate } from "@/lib/plan/format"

interface WhoIsHomeProps {
  /** The window's dates, in order. */
  dates: string[]
  slots: AvailabilityGridSlot[]
  presets: RoutinePreset[]
  /** Keyed `"<date>:<slotId>"`. */
  value: AvailabilityGridValue
  onChange: (value: AvailabilityGridValue) => void
  /** Eaters per date for days with guests (D16). */
  guests: Record<string, number>
  onGuestsChange: (guests: Record<string, number>) => void
  /** Eaters on a day without guests: where "Guests" starts from. */
  household: number
  asDefault: boolean
  onAsDefaultChange: (next: boolean) => void
  rangeLabel: string
  /** What the grid sits on: the dialog panel, or the page on phones. */
  surface: "ground" | "panel"
  disabled?: boolean
}

/**
 * Plan wizard step 1, "Who's home?" (UX §5.4): the window's dates × meals,
 * prefilled from what the cells resolve to. Each day's menu marks the day
 * away or adds guests; guests show as an eaters stepper per day below the
 * grid. Holds no state of its own.
 */
export function WhoIsHome({
  dates,
  slots,
  presets,
  value,
  onChange,
  guests,
  onGuestsChange,
  household,
  asDefault,
  onAsDefaultChange,
  rangeLabel,
  surface,
  disabled,
}: WhoIsHomeProps) {
  const { t, i18n } = useTranslation()
  const defaultId = useId()
  const hintId = useId()

  const longDay = (date: string) =>
    formatDate(date, i18n, { weekday: "long", day: "numeric", month: "long" })

  const columns: AvailabilityGridColumn[] = dates.map((date) => ({
    key: date,
    label: formatDate(date, i18n, { weekday: "short" }),
    fullLabel: longDay(date),
    sublabel: formatDate(date, i18n, { day: "numeric" }),
  }))

  function setDay(date: string, mode: "plan" | "none") {
    onChange({
      ...value,
      ...Object.fromEntries(slots.map((s) => [`${date}:${s.id}`, { mode }])),
    })
  }

  function addGuests(date: string) {
    if (date in guests) return
    onGuestsChange({ ...guests, [date]: household + 1 })
  }

  function removeGuests(date: string) {
    const next = { ...guests }
    delete next[date]
    onGuestsChange(next)
  }

  const guestDays = dates.filter((d) => d in guests)

  return (
    <div className="flex flex-col gap-6">
      <AvailabilityGrid
        label={t("plan.wizard.who.grid_label", { range: rangeLabel })}
        columns={columns}
        slots={slots}
        value={value}
        presets={presets}
        surface={surface}
        disabled={disabled}
        onChange={(date, slotId, cell) =>
          onChange({ ...value, [`${date}:${slotId}`]: cell })
        }
        columnActions={(date) => [
          {
            id: "away",
            label: t("plan.wizard.who.away"),
            onSelect: () => setDay(date, "none"),
          },
          {
            id: "plan",
            label: t("availability.bulk_plan"),
            onSelect: () => setDay(date, "plan"),
          },
          {
            id: "guests",
            label: t("plan.wizard.who.guests"),
            onSelect: () => addGuests(date),
          },
        ]}
      />

      {guestDays.length > 0 && (
        <section
          aria-labelledby={`${defaultId}-guests`}
          className="flex flex-col gap-2"
          data-testid="wizard-guests"
        >
          <h3
            id={`${defaultId}-guests`}
            className="font-sans text-body font-semibold text-ink"
          >
            {t("plan.wizard.who.guests_title")}
          </h3>
          <ul className="flex flex-col divide-y divide-line">
            {guestDays.map((date) => (
              <li
                key={date}
                className="flex flex-wrap items-center justify-between gap-2 py-2"
              >
                <span className="flex flex-col">
                  <span className="text-body text-ink">{longDay(date)}</span>
                  <span className="text-small text-ink-muted">
                    {t("plan.sheet.eaters")}
                  </span>
                </span>
                <span className="flex items-center gap-1">
                  <QuantityStepper
                    label={t("plan.wizard.who.eaters", { day: longDay(date) })}
                    value={guests[date]!}
                    min={0.5}
                    max={20}
                    step={0.5}
                    onChange={(n) => onGuestsChange({ ...guests, [date]: n })}
                    disabled={disabled}
                    testId={`wizard-guests-${date}`}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    className="size-11 md:pointer-fine:size-9"
                    aria-label={t("plan.wizard.who.remove_guests", {
                      day: longDay(date),
                    })}
                    disabled={disabled}
                    onClick={() => removeGuests(date)}
                  >
                    <X strokeWidth={1.5} aria-hidden />
                  </Button>
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}

      <div className="flex items-start gap-3">
        <input
          id={defaultId}
          type="checkbox"
          checked={asDefault}
          disabled={disabled}
          aria-describedby={hintId}
          onChange={(e) => onAsDefaultChange(e.target.checked)}
          className="mt-1 size-4 shrink-0 accent-accent pointer-coarse:size-5"
        />
        <div className="flex flex-col gap-0.5">
          <label htmlFor={defaultId} className="text-body text-ink">
            {t("plan.wizard.who.make_default")}
          </label>
          <p id={hintId} className="text-small text-ink-muted">
            {t("plan.wizard.who.make_default_hint")}
          </p>
        </div>
      </div>
    </div>
  )
}
