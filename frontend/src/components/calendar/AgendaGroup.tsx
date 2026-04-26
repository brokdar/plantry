import { parseISO } from "date-fns"
import { useTranslation } from "react-i18next"

import { CopyToCurrentButton } from "@/components/archive/CopyToCurrentButton"
import type { Plate } from "@/lib/api/plates"

interface AgendaGroupProps {
  weekLabel: string
  plates: Plate[]
  defaultOpen?: boolean
  foodsById?: Map<number, string>
  showCopyButton?: boolean
}

export function AgendaGroup({
  weekLabel,
  plates,
  defaultOpen = true,
  foodsById,
  showCopyButton = false,
}: AgendaGroupProps) {
  const { i18n } = useTranslation()
  const locale = i18n.language

  return (
    <details open={defaultOpen} className="group">
      <summary className="flex cursor-pointer list-none items-center gap-2 rounded-lg px-3 py-2 text-sm font-semibold text-on-surface select-none hover:bg-surface-container-low [&::-webkit-details-marker]:hidden">
        <span
          className="inline-block h-2 w-2 shrink-0 rounded-full border border-current opacity-60 transition-transform group-open:rotate-90"
          aria-hidden
        />
        {weekLabel}
        <span className="ml-auto text-xs font-normal text-on-surface-variant">
          {plates.length}
        </span>
        {showCopyButton && plates.length > 0 && (
          <CopyToCurrentButton
            weekId={plates[0].week_id}
            size="sm"
            iconOnly
            testId={`copy-to-current-agenda-${plates[0].week_id}`}
          />
        )}
      </summary>

      <ul className="mt-1 space-y-0.5 pl-4">
        {plates.map((plate) => {
          const date = parseISO(plate.date)
          const dayAbbr = new Intl.DateTimeFormat(locale, {
            weekday: "short",
          }).format(date)
          const formattedDate = new Intl.DateTimeFormat(locale, {
            month: "short",
            day: "numeric",
          }).format(date)
          const dishNames = plate.components
            .map((c) => foodsById?.get(c.food_id) ?? String(c.food_id))
            .join(", ")

          return (
            <li
              key={plate.id}
              className="flex items-center gap-3 rounded-md px-2 py-1.5 text-sm text-on-surface"
            >
              <span className="w-20 shrink-0 text-xs text-on-surface-variant">
                {formattedDate}
              </span>
              <span className="w-8 shrink-0 text-xs font-medium text-on-surface-variant">
                {dayAbbr}
              </span>
              <span className="w-8 shrink-0 text-xs text-on-surface-variant">
                #{plate.slot_id}
              </span>
              <span className="min-w-0 flex-1 truncate">
                {dishNames || "—"}
              </span>
            </li>
          )
        })}
      </ul>
    </details>
  )
}
