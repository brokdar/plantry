import { useNavigate } from "@tanstack/react-router"
import { X } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import type { AvailabilityGridValue } from "@/components/availability/AvailabilityGrid"
import { useIsPhone } from "@/components/plan/use-is-phone"
import { StepIndicator } from "@/components/setup/StepIndicator"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import type { GenerateInput } from "@/lib/api/generation"
import type { Plan } from "@/lib/api/plan"
import {
  useApplyWindowAvailability,
  type WindowCellChange,
} from "@/lib/queries/availability"
import { useGenerate } from "@/lib/queries/generation"
import { usePlan } from "@/lib/queries/plan"
import { usePresets } from "@/lib/queries/presets"
import { useProfile } from "@/lib/queries/profile"
import { useSettingItems } from "@/lib/queries/settings"
import { formatDate } from "@/lib/plan/format"
import { slotLabel } from "@/lib/slot-label"

import { AnythingSpecial } from "./AnythingSpecial"
import { ConfirmRemoval } from "./ConfirmRemoval"
import { Generating } from "./Generating"
import { WhoIsHome } from "./WhoIsHome"

const PRESET_PARAMS = { sort: "name", limit: 200 } as const
const STEP_COUNT = 3
const TITLES = [
  "plan.wizard.who.title",
  "plan.wizard.special.title",
  "plan.wizard.generating.title",
] as const

/** The window's cells as grid values keyed `"<date>:<slotId>"`. */
function cellsToValue(plan: Plan): AvailabilityGridValue {
  return Object.fromEntries(
    plan.cells.map((c) => [
      `${c.date}:${c.slot_id}`,
      c.mode === "routine" && c.preset_id !== null
        ? { mode: c.mode, presetId: c.preset_id }
        : { mode: c.mode },
    ])
  )
}

/**
 * `/plan/new` (UX §5.4, D31): Who's home? → Anything special? →
 * Generating, then the board of the window. A full page on phones, a
 * centred dialog over the board from 768 px. `start` undefined plans the
 * anchored window.
 */
export function PlanWizard({ start }: { start?: string }) {
  const { t } = useTranslation()
  const phone = useIsPhone()
  const navigate = useNavigate()
  const plan = usePlan(start)
  const settings = useSettingItems()
  const profile = useProfile()
  const presets = usePresets(PRESET_PARAMS)

  /** Back to the board; with a run it follows the generation (D35). */
  function close(run?: number) {
    void navigate({
      to: "/plan",
      search: { start: plan.data?.window.start ?? start, run },
    })
  }

  if (plan.isError) {
    return (
      <Frame phone={phone} title={t("plan.wizard.title")} onClose={close}>
        <p role="alert" className="text-body text-ink">
          {t("plan.wizard.error")}
        </p>
      </Frame>
    )
  }
  if (!plan.data || !settings.data || !profile.data) {
    return (
      <Frame phone={phone} title={t("plan.wizard.title")} onClose={close}>
        <div className="flex flex-col gap-3" aria-hidden>
          <Skeleton className="h-5 w-2/3" />
          <Skeleton className="h-48 w-full" />
        </div>
      </Frame>
    )
  }

  return (
    <WizardFlow
      key={plan.data.window.start}
      phone={phone}
      plan={plan.data}
      includeNewDefault={
        Number(settings.data["plan.explore_per_week"]?.value) > 0
      }
      household={profile.data.household_portions}
      // No provider configured: the heuristic plans (spec §13 "Without
      // AI"); with one, the server's default chooser (D11).
      assistant={Boolean(settings.data["ai.provider"]?.value)}
      presets={(presets.data?.items ?? []).map((p) => ({
        id: p.id,
        name: p.name,
      }))}
      onClose={close}
    />
  )
}

function WizardFlow({
  phone,
  plan,
  includeNewDefault,
  household,
  assistant,
  presets,
  onClose,
}: {
  phone: boolean
  plan: Plan
  includeNewDefault: boolean
  household: number
  assistant: boolean
  presets: { id: number; name: string }[]
  onClose: (run?: number) => void
}) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const apply = useApplyWindowAvailability()
  const generate = useGenerate({ openBoard: false })
  const [step, setStep] = useState(0)
  // The cells as the wizard opened, and as Next last wrote them (and
  // where to: overrides or defaults). Next writes only the difference.
  const [initial] = useState(() => cellsToValue(plan))
  const [saved, setSaved] = useState<{
    value: AvailabilityGridValue
    asDefault: boolean
  } | null>(null)
  const [value, setValue] = useState(initial)
  // Changes that would remove confirmed plates wait for the dialog.
  const [confirming, setConfirming] = useState<WindowCellChange[] | null>(null)
  const [guests, setGuests] = useState<Record<string, number>>({})
  const [asDefault, setAsDefault] = useState(false)
  const [instruction, setInstruction] = useState("")
  const [includeNew, setIncludeNew] = useState(includeNewDefault)
  const [runId, setRunId] = useState<number>()
  const [request, setRequest] = useState<GenerateInput | null>(null)
  const headingRef = useRef<HTMLHeadingElement>(null)

  const { window: win } = plan
  const dates = Array.from(new Set(plan.cells.map((c) => c.date))).sort()
  const slots = [...plan.slots]
    .sort((a, b) => a.sort_order - b.sort_order)
    .map((s) => ({ id: s.id, label: slotLabel(t, s.name_key) }))
  const rangeLabel = new Intl.DateTimeFormat(i18n.language, {
    day: "numeric",
    month: "short",
  }).formatRange(parseDate(win.start), parseDate(win.end))

  // Focus the new step's title so screen readers announce it.
  const firstRender = useRef(true)
  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false
      return
    }
    headingRef.current?.focus()
  }, [step])

  function next() {
    // Written to the other target (overrides vs defaults) before: compare
    // with the cells as they were, so every change lands on this target.
    const base = saved && saved.asDefault === asDefault ? saved.value : initial
    const changes: WindowCellChange[] = plan.cells.flatMap((c) => {
      const key = `${c.date}:${c.slot_id}`
      const now = value[key] ?? { mode: "plan" }
      const was = base[key] ?? { mode: "plan" }
      if (now.mode === was.mode && now.presetId === was.presetId) return []
      return [
        {
          date: c.date,
          slotId: c.slot_id,
          mode: now.mode,
          presetId: now.presetId ?? null,
          cell: c,
        },
      ]
    })
    if (changes.length === 0) {
      setStep(1)
      return
    }
    // Overrides remove a No plate cell's plate; a confirmed one may be
    // bought already, so that asks first (D34). Drafts go without asking.
    if (!asDefault && confirmedRemovals(changes).length > 0) {
      setConfirming(changes)
      return
    }
    write(changes)
  }

  function write(changes: WindowCellChange[]) {
    apply.mutate(
      { changes, asDefault, slotIds: slots.map((s) => s.id) },
      {
        onSuccess: () => {
          setSaved({ value, asDefault })
          setConfirming(null)
          setStep(1)
        },
      }
    )
  }

  /** "Thursday dinner: Chili sin carne" for each confirmed plate a change
   *  removes. */
  function confirmedRemovals(changes: WindowCellChange[]) {
    return changes.flatMap(({ mode, cell }) =>
      mode === "none" && cell.plate?.state === "confirmed"
        ? [
            t("plan.wizard.remove.item", {
              day: formatDate(cell.date, i18n, { weekday: "long" }),
              slot: slots.find((s) => s.id === cell.slot_id)?.label ?? "",
              food: cell.plate.components.map((p) => p.food_name).join(", "),
            }),
          ]
        : []
    )
  }

  function start(input: GenerateInput) {
    setRequest(input)
    generate.mutate(input, {
      onSuccess: ({ run_id }) => {
        setRunId(run_id)
        setStep(2)
      },
    })
  }

  function startWith(note: string) {
    const eaters = Object.keys(guests).length > 0 ? guests : undefined
    start({
      start: win.start,
      instruction: note.trim() || undefined,
      eaters_by_date: eaters,
      include_new_recipe: includeNew,
      ...(assistant ? {} : { chooser: "heuristic" as const }),
    })
  }

  const busy = apply.isPending || generate.isPending

  return (
    <Frame
      phone={phone}
      title={t(TITLES[step])}
      headingRef={headingRef}
      indicator={<StepIndicator step={step + 1} total={STEP_COUNT} />}
      onClose={() => onClose(runId)}
    >
      {step === 0 && (
        <>
          <p className="text-body text-ink-muted">
            {t("plan.wizard.who.body")}
          </p>
          {slots.length === 0 ? (
            <p className="text-body text-ink">
              {t("plan.wizard.who.no_slots")}
            </p>
          ) : (
            <WhoIsHome
              dates={dates}
              slots={slots}
              presets={presets}
              value={value}
              onChange={setValue}
              guests={guests}
              onGuestsChange={setGuests}
              household={household}
              asDefault={asDefault}
              onAsDefaultChange={setAsDefault}
              rangeLabel={rangeLabel}
              surface={phone ? "ground" : "panel"}
              disabled={busy}
            />
          )}
          <Footer>
            <Button
              type="button"
              size="lg"
              className="ml-auto min-w-24 pointer-coarse:h-11"
              disabled={busy || slots.length === 0}
              onClick={next}
            >
              {t("plan.wizard.next")}
            </Button>
          </Footer>
        </>
      )}
      {step === 1 && (
        <>
          <p className="text-body text-ink-muted">
            {/* The heuristic doesn't read the note (D11). */}
            {assistant
              ? t("plan.wizard.special.body")
              : t("plan.wizard.special.body_heuristic")}
          </p>
          <AnythingSpecial
            instruction={instruction}
            onInstructionChange={setInstruction}
            includeNew={includeNew}
            onIncludeNewChange={setIncludeNew}
          />
          <Footer>
            <Button
              type="button"
              variant="outline"
              size="lg"
              className="pointer-coarse:h-11"
              disabled={busy}
              onClick={() => setStep(0)}
            >
              {t("plan.wizard.back")}
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="lg"
              className="ml-auto pointer-coarse:h-11"
              disabled={busy}
              onClick={() => startWith("")}
            >
              {t("plan.wizard.skip")}
            </Button>
            <Button
              type="button"
              size="lg"
              className="pointer-coarse:h-11"
              disabled={busy}
              onClick={() => startWith(instruction)}
            >
              {t("nav.generate_plan")}
            </Button>
          </Footer>
        </>
      )}
      {step === 2 && runId !== undefined && request && (
        <Generating
          key={runId}
          runId={runId}
          pending={generate.isPending}
          onDone={(run) =>
            void navigate({
              to: "/plan",
              search: { start: win.start, run: run.id },
              viewTransition: true,
            })
          }
          onRetry={() => start(request)}
          onWithoutAssistant={() => start({ ...request, chooser: "heuristic" })}
        />
      )}
      <ConfirmRemoval
        plates={confirming ? confirmedRemovals(confirming) : []}
        pending={apply.isPending}
        onCancel={() => setConfirming(null)}
        onConfirm={() => confirming && write(confirming)}
      />
    </Frame>
  )
}

function parseDate(iso: string) {
  const [y, m, d] = iso.split("-").map(Number)
  return new Date(y, m - 1, d)
}

function Footer({ children }: { children: React.ReactNode }) {
  return (
    <footer className="flex flex-wrap items-center gap-2 border-t border-line pt-4">
      {children}
    </footer>
  )
}

/** A full page on phones; a centred dialog over the board from 768 px. */
function Frame({
  phone,
  title,
  headingRef,
  indicator,
  onClose,
  children,
}: {
  phone: boolean
  title: string
  headingRef?: React.RefObject<HTMLHeadingElement | null>
  indicator?: React.ReactNode
  onClose: () => void
  children: React.ReactNode
}) {
  const { t } = useTranslation()
  const header = (heading: React.ReactNode) => (
    <header className="flex flex-col gap-4 pr-10">
      {indicator}
      {heading}
    </header>
  )
  const headingClass = "font-heading text-h2 text-ink outline-none"

  if (phone) {
    return (
      <div
        className="relative mx-auto flex w-full max-w-[560px] flex-col gap-6 px-4 py-6"
        data-testid="plan-wizard"
      >
        <Button
          type="button"
          variant="ghost"
          className="absolute top-4 right-2 size-11"
          aria-label={t("common.close")}
          onClick={onClose}
        >
          <X strokeWidth={1.5} aria-hidden />
        </Button>
        {header(
          <h1 ref={headingRef} tabIndex={-1} className={headingClass}>
            {title}
          </h1>
        )}
        {children}
      </div>
    )
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent
        aria-describedby={undefined}
        // Start at the title, not on the first day's menu.
        onOpenAutoFocus={(e) => {
          if (!headingRef) return
          e.preventDefault()
          headingRef.current?.focus()
        }}
        data-testid="plan-wizard"
        className="flex max-h-[min(90svh,56rem)] flex-col gap-6 overflow-y-auto rounded-sheet bg-panel p-6 text-ink shadow-sheet sm:max-w-[46rem]"
      >
        {header(
          <DialogTitle ref={headingRef} tabIndex={-1} className={headingClass}>
            {title}
          </DialogTitle>
        )}
        {children}
      </DialogContent>
    </Dialog>
  )
}
