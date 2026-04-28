import { Sparkles } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { useChatUI } from "@/lib/stores/chat-ui"
import { usePlannerUI } from "@/lib/stores/planner-ui"

interface FillEmptySlotsButtonProps {
  rangeFrom?: string
  rangeTo?: string
}

// Triggers the kitchen-agent's fill_empty mode through the chat panel. The
// existing agent tool-set respects skipped plates and biases toward favorites
// (see backend agent/service.go modeHint). Tracking of which plates the agent
// just created happens client-side via an effect in the planner page so the
// AI-filled gold badge only renders for this session.
export function FillEmptySlotsButton({
  rangeFrom = "",
  rangeTo = "",
}: FillEmptySlotsButtonProps) {
  const { t } = useTranslation()
  const startAiFill = usePlannerUI((s) => s.startAiFill)
  const openWith = useChatUI((s) => s.openWith)
  const setMode = useChatUI((s) => s.setMode)

  function handleClick() {
    startAiFill({ from: rangeFrom, to: rangeTo })
    setMode("fill_empty")
    openWith(t("planner.fill_empty.progress"))
  }

  return (
    <Button
      onClick={handleClick}
      data-testid="fill-empty-slots"
      className="gap-1.5"
    >
      <Sparkles className="size-4" aria-hidden />
      {t("planner.fill_empty_cta")}
    </Button>
  )
}
