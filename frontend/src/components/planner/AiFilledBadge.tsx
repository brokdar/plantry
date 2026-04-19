import { Sparkles } from "lucide-react"
import { useTranslation } from "react-i18next"

export function AiFilledBadge() {
  const { t } = useTranslation()
  return (
    <span
      className="absolute top-2 left-2 grid size-[22px] place-items-center rounded-full bg-[#c2974a] text-white shadow-[0_2px_6px_rgba(194,151,74,0.4)]"
      title={t("planner.slot.ai_filled")}
      aria-label={t("planner.slot.ai_filled")}
      data-testid="slot-ai-filled"
    >
      <Sparkles className="h-2.5 w-2.5" aria-hidden />
    </span>
  )
}
