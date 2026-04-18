import { useNavigate } from "@tanstack/react-router"
import { Sparkles } from "lucide-react"
import { useTranslation } from "react-i18next"

import { useChatUI } from "@/lib/stores/chat-ui"
import { cn } from "@/lib/utils"

export type GeneratePlanVariant = "default" | "rail" | "fab"

type GeneratePlanButtonProps = {
  variant?: GeneratePlanVariant
}

const VARIANT_CLASSES: Record<
  GeneratePlanVariant,
  {
    root: string
    icon: string
    showLabel: boolean
    testId: string
  }
> = {
  default: {
    root: "gradient-primary editorial-shadow flex w-full items-center justify-center gap-2 rounded-lg px-4 py-3 text-sm font-bold text-on-primary transition-transform active:scale-95",
    icon: "size-4",
    showLabel: true,
    testId: "generate-plan-default",
  },
  rail: {
    root: "gradient-primary flex size-10 items-center justify-center rounded-full text-on-primary shadow-md transition-opacity active:opacity-80",
    icon: "size-5",
    showLabel: false,
    testId: "generate-plan-rail",
  },
  fab: {
    root: "gradient-primary editorial-shadow flex size-14 items-center justify-center rounded-full text-on-primary transition-transform active:scale-95",
    icon: "size-6",
    showLabel: false,
    testId: "generate-plan-fab",
  },
}

export function GeneratePlanButton({
  variant = "default",
}: GeneratePlanButtonProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const openWith = useChatUI((s) => s.openWith)
  const label = t("nav.generate_plan")
  const { root, icon, showLabel, testId } = VARIANT_CLASSES[variant]

  async function handleClick() {
    await navigate({ to: "/" })
    openWith(t("planner.generate_prompt"))
  }

  return (
    <button
      type="button"
      onClick={handleClick}
      aria-label={showLabel ? undefined : label}
      className={cn(root)}
      data-testid={testId}
    >
      <Sparkles className={icon} aria-hidden />
      {showLabel && label}
    </button>
  )
}
