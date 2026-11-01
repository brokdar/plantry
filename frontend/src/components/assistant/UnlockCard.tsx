import { LockOpen } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import type { PlanSlot } from "@/lib/api/plan"
import type { UnlockRequestPayload } from "@/lib/domain/assistantEvents"
import { slotInSentence } from "@/lib/assistant/change-text"
import { slotLabel } from "@/lib/slot-label"

interface UnlockCardProps {
  request: UnlockRequestPayload
  slots: PlanSlot[]
  disabled: boolean
  /** Sends the confirmation text as the user's next message. */
  onConfirm: (text: string) => void
}

/** The agent wants to change confirmed plates (spec §7.6): lists them and
 *  lets the user answer yes with one tap, which sends that text. */
export function UnlockCard({
  request,
  slots,
  disabled,
  onConfirm,
}: UnlockCardProps) {
  const { t, i18n } = useTranslation()
  const confirm = t("assistant.unlock.confirm")
  return (
    <div
      data-testid="assistant-unlock-card"
      className="space-y-2 rounded-cell border border-line bg-panel px-3 py-3 text-small text-ink"
    >
      <p className="flex items-center gap-2 font-semibold">
        <LockOpen className="size-4 shrink-0" aria-hidden="true" />
        {t("assistant.unlock.title")}
      </p>
      <ul className="list-inside list-disc text-ink">
        {request.rows.map((r) => {
          const slot = slots.find((s) => s.id === r.slot_id)
          const [y, m, d] = r.date.split("-").map(Number)
          return (
            <li key={`${r.date}|${r.slot_id}`}>
              {t("assistant.unlock.row", {
                day: new Intl.DateTimeFormat(i18n.language, {
                  weekday: "long",
                }).format(new Date(y, m - 1, d)),
                slot: slot
                  ? slotInSentence(slotLabel(t, slot.name_key), i18n.language)
                  : "",
                names: r.names.join(", "),
              })}
            </li>
          )
        })}
      </ul>
      <p className="text-ink-muted">{t("assistant.unlock.body")}</p>
      <Button
        type="button"
        size="sm"
        disabled={disabled}
        onClick={() => onConfirm(confirm)}
        data-testid="assistant-unlock-confirm"
      >
        {confirm}
      </Button>
    </div>
  )
}
