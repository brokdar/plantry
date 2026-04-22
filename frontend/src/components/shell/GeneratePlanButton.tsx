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
    root: "btn-shimmer group gradient-primary editorial-shadow flex w-full items-center justify-center gap-2 rounded-lg px-4 py-3 text-sm font-bold text-on-primary transition-[transform,box-shadow] duration-150 ease-out hover:-translate-y-0.5 hover:shadow-[0_6px_20px_-4px_rgba(74,101,77,0.50)] active:translate-y-0 active:scale-[0.98] active:shadow-none",
    icon: "size-4 transition-transform duration-300 group-hover:rotate-12",
    showLabel: true,
    testId: "generate-plan-default",
  },
  rail: {
    root: "gradient-primary group flex size-10 items-center justify-center rounded-full text-on-primary shadow-md transition-[transform,box-shadow,opacity] duration-150 ease-out hover:scale-110 hover:shadow-[0_4px_14px_-2px_rgba(74,101,77,0.50)] active:scale-95 active:opacity-80",
    icon: "size-5 transition-transform duration-300 group-hover:rotate-12",
    showLabel: false,
    testId: "generate-plan-rail",
  },
  fab: {
    root: "btn-shimmer group gradient-primary editorial-shadow flex size-14 items-center justify-center rounded-full text-on-primary transition-[transform,box-shadow] duration-150 ease-out hover:scale-105 hover:shadow-[0_8px_24px_-4px_rgba(74,101,77,0.55)] active:scale-95",
    icon: "size-6 transition-transform duration-300 group-hover:rotate-12",
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
