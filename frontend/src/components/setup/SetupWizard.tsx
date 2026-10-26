import { useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"
import { useAvailabilityDefaults } from "@/lib/queries/availability"
import { useProfile } from "@/lib/queries/profile"
import { useSettingItems } from "@/lib/queries/settings"
import { useTimeSlots } from "@/lib/queries/slots"

import { AssistantStep } from "./AssistantStep"
import { CookingStep } from "./CookingStep"
import { FinishStep } from "./FinishStep"
import { HouseholdStep } from "./HouseholdStep"
import { MealsStep } from "./MealsStep"
import { NutritionStep } from "./NutritionStep"
import { StepIndicator } from "./StepIndicator"
import { STEP_COUNT, useWizardSave } from "./use-wizard-save"
import { VarietyStep } from "./VarietyStep"
import { WelcomeStep } from "./WelcomeStep"
import { buildDraft, mealsError, type WizardDraft } from "./wizard-draft"

/** i18n key segment per step, in order. */
const STEP_IDS = [
  "welcome",
  "household",
  "nutrition",
  "meals",
  "cooking",
  "variety",
  "assistant",
] as const

/**
 * The first-run wizard (`/setup`). Loads the stored values once, then walks
 * seven steps; each Next saves its step. Re-running it opens prefilled.
 */
export function SetupWizard() {
  const { i18n } = useTranslation()
  const profile = useProfile()
  const settings = useSettingItems()
  const slots = useTimeSlots(false)
  const defaults = useAvailabilityDefaults()

  if (!profile.data || !settings.data || !slots.data || !defaults.data) {
    return (
      <WizardColumn>
        <div className="flex flex-col gap-4" aria-hidden>
          <Skeleton className="h-4 w-24" />
          <Skeleton className="h-8 w-3/4" />
          <Skeleton className="h-40 w-full" />
        </div>
      </WizardColumn>
    )
  }

  return (
    <WizardFlow
      initial={buildDraft({
        language: i18n.language,
        profile: profile.data,
        settings: settings.data,
        slots: slots.data.items,
        defaults: defaults.data.items,
      })}
    />
  )
}

function WizardColumn({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-svh bg-ground text-ink">
      <main
        id="main"
        className="mx-auto flex w-full max-w-[560px] flex-col gap-8 px-4 py-8 md:py-16"
      >
        {children}
      </main>
    </div>
  )
}

function WizardFlow({ initial }: { initial: WizardDraft }) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState(initial)
  const [step, setStep] = useState(0)
  const [saving, setSaving] = useState(false)
  const [blocked, setBlocked] = useState<string | null>(null)
  const { saveStep, skipFrom } = useWizardSave()
  const headingRef = useRef<HTMLHeadingElement>(null)
  const finished = step === STEP_COUNT

  // Move focus to the new step's title so screen readers announce it.
  const firstRender = useRef(true)
  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false
      return
    }
    headingRef.current?.focus()
  }, [step])

  function update<K extends keyof WizardDraft>(key: K) {
    return (value: WizardDraft[K]) => {
      setBlocked(null)
      setDraft((d) => ({ ...d, [key]: value }))
    }
  }

  /** Runs a save; on failure the mutation has toasted and we stay put. */
  async function run(save: () => Promise<void>) {
    if (step === 3) {
      const error = mealsError(draft.meals)
      if (error) {
        setBlocked(t(`setup.meals.error_${error}`))
        return
      }
    }
    setSaving(true)
    try {
      await save()
    } catch {
      // The failed mutation already showed its toast.
    } finally {
      setSaving(false)
    }
  }

  const next = () =>
    run(async () => {
      const stored = await saveStep(step, draft)
      setDraft(stored)
      setStep((s) => s + 1)
    })

  const skip = () =>
    run(async () => {
      await skipFrom(step, draft)
      setStep(STEP_COUNT)
    })

  if (finished) {
    return (
      <WizardColumn>
        <FinishStep headingRef={headingRef} />
      </WizardColumn>
    )
  }

  const id = STEP_IDS[step]!
  // On the last step Skip would do the same as Next.
  const showSkip = step > 0 && step < STEP_COUNT - 1

  return (
    <WizardColumn>
      <header className="flex flex-col gap-4">
        <StepIndicator step={step + 1} total={STEP_COUNT} />
        <div className="flex flex-col gap-2">
          <h1 ref={headingRef} tabIndex={-1} className="outline-none">
            {t(`setup.${id}.title`)}
          </h1>
          <p className="text-body text-ink-muted">{t(`setup.${id}.why`)}</p>
        </div>
      </header>

      <section aria-label={t(`setup.${id}.title`)}>
        {step === 0 && (
          <WelcomeStep value={draft.language} onChange={update("language")} />
        )}
        {step === 1 && (
          <HouseholdStep
            value={draft.household}
            onChange={update("household")}
          />
        )}
        {step === 2 && (
          <NutritionStep
            value={draft.nutrition}
            onChange={update("nutrition")}
          />
        )}
        {step === 3 && (
          <MealsStep value={draft.meals} onChange={update("meals")} />
        )}
        {step === 4 && (
          <CookingStep value={draft.cooking} onChange={update("cooking")} />
        )}
        {step === 5 && (
          <VarietyStep value={draft.variety} onChange={update("variety")} />
        )}
        {step === 6 && (
          <AssistantStep
            value={draft.assistant}
            onChange={update("assistant")}
          />
        )}
      </section>

      <footer className="flex flex-col gap-3 border-t border-line pt-6">
        {blocked && (
          <p role="alert" className="text-small text-danger">
            {blocked}
          </p>
        )}
        {/* Phone: Back | Next on one row, Skip right-aligned below it.
            From sm: Back … Skip Next on one row. */}
        <div className="grid grid-cols-2 items-center gap-2 sm:flex">
          {step > 0 && (
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setBlocked(null)
                setStep((s) => s - 1)
              }}
              disabled={saving}
              size="lg"
              className="justify-self-start pointer-coarse:h-11"
            >
              {t("setup.back")}
            </Button>
          )}
          {showSkip && (
            <Button
              type="button"
              variant="link"
              onClick={skip}
              disabled={saving}
              size="lg"
              className="col-span-2 row-start-2 justify-self-end px-0 sm:ml-auto sm:px-2.5 pointer-coarse:h-11"
              data-testid="setup-skip"
            >
              {t("setup.skip")}
            </Button>
          )}
          <Button
            type="button"
            onClick={next}
            disabled={saving}
            size="lg"
            className={cn(
              "col-start-2 row-start-1 min-w-24 justify-self-end pointer-coarse:h-11",
              !showSkip && "sm:ml-auto"
            )}
            data-testid="setup-next"
          >
            {t("setup.next")}
          </Button>
        </div>
      </footer>
    </WizardColumn>
  )
}
