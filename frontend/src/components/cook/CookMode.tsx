import { Snowflake } from "lucide-react"
import { Tabs as TabsPrimitive } from "radix-ui"
import { useState, type ReactNode } from "react"
import { useTranslation } from "react-i18next"

import { QuantityStepper } from "@/components/plan/QuantityStepper"
import { Button } from "@/components/ui/button"
import type { ScaledIngredient } from "@/lib/api/foods"
import type { DetectedTimer } from "@/lib/cook/timers"
import { formatAmount } from "@/lib/plan/format"

import { ScaledIngredients } from "./ScaledIngredients"
import { StepPager } from "./StepPager"
import { TimerPill, type RunningTimer } from "./TimerPill"
import { useWakeLock } from "./useWakeLock"

/** Batch and recipe portions: whole steps up to the scaling limit. */
const MIN_PORTIONS = 1
const MAX_PORTIONS = 100

export interface CookModeProps {
  name: string
  /** Step texts in order. */
  steps: readonly string[]
  /** Scaled to `portions`; undefined while loading. */
  ingredients: readonly ScaledIngredient[] | undefined
  portions: number
  onPortionsChange: (portions: number) => void
  /** Session mode: "Covers Sun dinner, Mon lunch · 2 spare". */
  coverage?: string
  /** Session mode: portions to freeze, reminded on the last step. */
  freezePortions?: number
  /** The way out, e.g. a link back to Today or the recipe. */
  close: ReactNode
  onDone: () => void
  donePending?: boolean
}

const TAB =
  "inline-flex h-11 flex-1 items-center justify-center rounded-lg text-h3 text-ink-muted transition-colors duration-150 ease-out data-[state=active]:bg-panel data-[state=active]:font-semibold data-[state=active]:text-ink"
// Phone: one tab at a time. Tablet and up: side by side, so both stay shown.
const PANEL =
  "flex min-w-0 flex-col gap-4 data-[state=inactive]:hidden md:data-[state=inactive]:flex"

/**
 * Cook mode (UX §5.3): full screen and large type for cooking at arm's
 * length. The header holds the name, "Cooking × 6", the portion stepper and
 * any running timers; below, the scaled ingredients and the step pager
 * are tabs on a phone and side by side from 768 px. Keeps the screen on.
 */
export function CookMode({
  name,
  steps,
  ingredients,
  portions,
  onPortionsChange,
  coverage,
  freezePortions = 0,
  close,
  onDone,
  donePending,
}: CookModeProps) {
  const { t, i18n } = useTranslation()
  const [tab, setTab] = useState("ingredients")
  const [timers, setTimers] = useState<RunningTimer[]>([])
  useWakeLock()

  function startTimer(timer: DetectedTimer) {
    setTimers((prev) => [
      ...prev,
      {
        id: (prev.at(-1)?.id ?? 0) + 1,
        label: timer.text,
        endsAt: Date.now() + timer.seconds * 1000,
      },
    ])
  }

  const finish = (
    <Button
      type="button"
      className="h-14 w-full text-h3"
      disabled={donePending}
      onClick={onDone}
      data-testid="cook-done"
    >
      {t("cook.done")}
    </Button>
  )

  return (
    <div className="flex min-h-dvh flex-col bg-ground text-h3 text-ink">
      <header className="sticky top-0 z-10 border-b border-line bg-ground">
        <div className="mx-auto flex w-full max-w-6xl flex-col gap-3 px-4 py-3 md:px-8">
          <div className="flex items-start gap-3">
            <h1 className="min-w-0 flex-1 text-h1 font-semibold break-words text-ink">
              {name}
            </h1>
            {close}
          </div>
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
            <span
              className="inline-flex items-center rounded-tag bg-ink px-2 py-0.5 text-small font-semibold text-ground tabular-nums"
              data-testid="cook-badge"
            >
              {t("cook.cooking", { portions: formatAmount(portions, i18n) })}
            </span>
            <div className="flex items-center gap-2">
              <span className="text-small text-ink-muted">
                {t("cook.portions")}
              </span>
              <QuantityStepper
                label={t("cook.portions")}
                value={portions}
                min={MIN_PORTIONS}
                max={MAX_PORTIONS}
                step={1}
                onChange={onPortionsChange}
                testId="cook-portions"
              />
            </div>
          </div>
          {coverage && (
            <p
              className="text-small text-ink-muted"
              data-testid="cook-coverage"
            >
              {coverage}
            </p>
          )}
          {timers.length > 0 && (
            <div
              role="group"
              aria-label={t("cook.timer.group")}
              className="-mx-4 flex gap-2 overflow-x-auto px-4 md:mx-0 md:flex-wrap md:px-0"
            >
              {timers.map((timer) => (
                <TimerPill
                  key={timer.id}
                  timer={timer}
                  onDismiss={() =>
                    setTimers((prev) => prev.filter((x) => x.id !== timer.id))
                  }
                />
              ))}
            </div>
          )}
        </div>
      </header>

      <TabsPrimitive.Root
        value={tab}
        onValueChange={setTab}
        className="mx-auto flex w-full max-w-6xl flex-1 flex-col gap-4 px-4 py-4 md:px-8 md:py-6"
      >
        <TabsPrimitive.List
          aria-label={name}
          className="flex gap-1 rounded-lg bg-panel-sunken p-1 md:hidden"
        >
          <TabsPrimitive.Trigger value="ingredients" className={TAB}>
            {t("cook.tabs.ingredients")}
          </TabsPrimitive.Trigger>
          <TabsPrimitive.Trigger value="steps" className={TAB}>
            {t("cook.tabs.steps")}
          </TabsPrimitive.Trigger>
        </TabsPrimitive.List>
        <div className="flex flex-1 flex-col md:grid md:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] md:gap-12">
          <TabsPrimitive.Content
            value="ingredients"
            forceMount
            className={PANEL}
          >
            <h2 className="hidden text-h2 font-semibold md:block">
              {t("cook.tabs.ingredients")}
            </h2>
            <ScaledIngredients ingredients={ingredients} />
          </TabsPrimitive.Content>
          <TabsPrimitive.Content
            value="steps"
            forceMount
            className={`${PANEL} flex-1`}
          >
            <h2 className="hidden text-h2 font-semibold md:block">
              {t("cook.tabs.steps")}
            </h2>
            <StepPager
              steps={steps}
              onStartTimer={startTimer}
              finish={finish}
              lastStepNote={
                freezePortions > 0 && (
                  <p
                    className="inline-flex items-center gap-2 self-start rounded-lg border border-line bg-panel px-3 py-2 text-h3 text-ink"
                    data-testid="cook-freeze"
                  >
                    <Snowflake
                      className="size-5"
                      strokeWidth={1.5}
                      aria-hidden
                    />
                    {t("cook.freeze", {
                      count: freezePortions,
                      portions: formatAmount(freezePortions, i18n),
                    })}
                  </p>
                )
              }
            />
          </TabsPrimitive.Content>
        </div>
      </TabsPrimitive.Root>
    </div>
  )
}
