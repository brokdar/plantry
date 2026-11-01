import { useId, useState } from "react"
import { Link, useNavigate, useRouterState } from "@tanstack/react-router"
import { Sparkles } from "lucide-react"
import { useTranslation } from "react-i18next"

import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { useSetupStatus } from "@/lib/queries/setup"
import { cn } from "@/lib/utils"

/** `default`: labelled, in the wide rail. `rail`: icon, in the narrow rail.
 *  `compact`: icon, in the phone header of `/plan` (D31). */
export type GeneratePlanVariant = "default" | "rail" | "compact"

type GeneratePlanButtonProps = {
  variant?: GeneratePlanVariant
}

const BASE =
  "inline-flex items-center justify-center rounded-lg bg-accent text-on-accent transition-colors duration-150 ease-out hover:bg-accent/90 focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-ground focus-visible:outline-none aria-disabled:cursor-not-allowed aria-disabled:opacity-50 aria-disabled:hover:bg-accent"

const VARIANTS: Record<
  GeneratePlanVariant,
  {
    root: string
    showLabel: boolean
    testId: string
    tooltipSide: "top" | "right" | "bottom"
  }
> = {
  default: {
    root: "h-11 w-full gap-2 px-4 text-body font-semibold",
    showLabel: true,
    testId: "generate-plan-default",
    tooltipSide: "top",
  },
  rail: {
    root: "size-11",
    showLabel: false,
    testId: "generate-plan-rail",
    tooltipSide: "right",
  },
  compact: {
    root: "size-11",
    showLabel: false,
    testId: "generate-plan-compact",
    tooltipSide: "bottom",
  },
}

/**
 * Plan decision D19: always visible, but `aria-disabled` (still focusable)
 * while the library is below the recipe target. The tooltip explains why on
 * hover, focus and tap; the default variant also shows a link to Recipes.
 */
export function GeneratePlanButton({
  variant = "default",
}: GeneratePlanButtonProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  // On the board, the wizard plans the window it shows.
  const boardStart = useRouterState({
    select: (s) =>
      /^\/plan\/?$/.test(s.location.pathname)
        ? (s.location.search as { start?: string }).start
        : undefined,
  })
  const { data: setup } = useSetupStatus()
  const [tipOpen, setTipOpen] = useState(false)
  const reasonId = useId()

  const label = t("nav.generate_plan")
  const { root, showLabel, testId, tooltipSide } = VARIANTS[variant]
  // Locked until the status loads; the reason shows only once it is known.
  const missing = setup
    ? Math.max(0, setup.recipe_target - setup.recipe_count)
    : 0
  const short = missing > 0
  const locked = !setup || short
  const reason = t("nav.generate_locked", { count: missing })

  function handleClick(e: React.MouseEvent<HTMLButtonElement>) {
    if (locked) {
      // Keeps Radix from closing the tooltip on click, so a tap shows it.
      e.preventDefault()
      setTipOpen(true)
      return
    }
    // The plan wizard (D32); the chat stays behind Ask assistant.
    void navigate({ to: "/plan/new", search: { start: boardStart } })
  }

  const button = (
    <TooltipProvider delayDuration={200}>
      <Tooltip open={short && tipOpen} onOpenChange={setTipOpen}>
        <TooltipTrigger asChild>
          <button
            type="button"
            onClick={handleClick}
            aria-label={showLabel ? undefined : label}
            aria-disabled={locked || undefined}
            aria-describedby={short ? reasonId : undefined}
            className={cn(BASE, root)}
            data-testid={testId}
          >
            <Sparkles
              className={showLabel ? "size-4" : "size-5"}
              strokeWidth={1.5}
              aria-hidden
            />
            {showLabel && label}
          </button>
        </TooltipTrigger>
        {short && <TooltipContent side={tooltipSide}>{reason}</TooltipContent>}
      </Tooltip>
    </TooltipProvider>
  )

  const description = short && (
    <span id={reasonId} className="sr-only">
      {reason}
    </span>
  )

  if (!showLabel) {
    return (
      <>
        {button}
        {description}
      </>
    )
  }

  return (
    <div className="flex w-full flex-col items-center gap-1">
      {button}
      {description}
      {short && (
        <Link
          to="/recipes"
          className="inline-flex min-h-11 items-center text-center text-small text-accent underline underline-offset-2 hover:no-underline focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none"
          data-testid="generate-plan-locked-link"
        >
          {reason}
        </Link>
      )}
    </div>
  )
}
