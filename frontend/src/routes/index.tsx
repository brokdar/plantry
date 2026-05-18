import {
  BarChart2,
  Bookmark,
  BookmarkPlus,
  CalendarRange,
  Download,
  FileDown,
  Keyboard,
  LayoutList,
  MoreHorizontal,
  Settings,
  Sparkles,
  Trash2,
} from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { createFileRoute, Link } from "@tanstack/react-router"

import { ChatPanel } from "@/components/chat/ChatPanel"
import { ShortcutCheatsheet } from "@/components/planner/ShortcutCheatsheet"
import { CopyFromWeekDialog } from "@/components/presets/CopyFromWeekDialog"
import { SaveAsTemplateDialog } from "@/components/templates/SaveAsTemplateDialog"
import { TemplatePicker } from "@/components/templates/TemplatePicker"
import {
  showApplyToasts,
  snapshotOverwrittenPlates,
} from "@/lib/template-apply-toast"
import { PageHeader } from "@/components/editorial/PageHeader"
import { DateRangeNavigator } from "@/components/planner/DateRangeNavigator"
import { EmptyWeekCTA } from "@/components/planner/EmptyWeekCTA"
import { MobilePlannerGrid } from "@/components/planner/MobilePlannerGrid"
import { NutritionWeekSummary } from "@/components/planner/NutritionWeekSummary"
import { PlannerGrid, type PlannerDay } from "@/components/planner/PlannerGrid"
import { RevertBanner } from "@/components/planner/RevertBanner"
import { ShoppingPanel } from "@/components/planner/ShoppingPanel"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import {
  addPlateComponent,
  componentToAddInput,
  createPlate,
  deletePlate,
  listPlates,
  type Plate,
} from "@/lib/api/plates"
import { isCheatsheetShortcut } from "@/lib/planner-keynav"
import {
  computeAnchor,
  shiftYMD,
  windowRange,
  type AnchorMode,
} from "@/lib/planner-window"
import { useAISettings } from "@/lib/queries/ai"
import { queryClient } from "@/lib/query-client"
import { plateKeys } from "@/lib/queries/keys"
import { usePlatesRange } from "@/lib/queries/plates"
import { useSettings } from "@/lib/queries/settings"
import { useTimeSlots } from "@/lib/queries/slots"
import { useNutritionRange } from "@/lib/queries/nutrition"
import { useChatUI } from "@/lib/stores/chat-ui"
import { usePlannerUI } from "@/lib/stores/planner-ui"
import { toast, toastError } from "@/lib/toast"

export const Route = createFileRoute("/")({
  validateSearch: (search): { date?: string } => ({
    date: (search.date as string) ?? undefined,
  }),
  component: PlanPage,
})

function PlanPage() {
  const { t, i18n } = useTranslation()
  const { date: dateParam } = Route.useSearch()
  // If ?date= is provided, compute initial offset so the window starts at that date.
  const [windowOffset, setWindowOffset] = useState(() => {
    if (!dateParam) return 0
    const target = new Date(dateParam + "T00:00:00")
    const today = new Date()
    const diff = Math.round(
      (target.getTime() - today.getTime()) / (1000 * 60 * 60 * 24)
    )
    // Round to nearest 7-day boundary (floor toward past)
    return Math.floor(diff / 7) * 7
  })
  const [shoppingOpen, setShoppingOpen] = useState(false)
  const [nutritionOpen, setNutritionOpen] = useState(false)
  const [saveRangeOpen, setSaveRangeOpen] = useState(false)
  const [applyWeekOpen, setApplyWeekOpen] = useState(false)
  const [copyWeekOpen, setCopyWeekOpen] = useState(false)
  const [cheatsheetOpen, setCheatsheetOpen] = useState(false)

  // Global `?` shortcut. Lives at the route level so the toolbar menu item
  // and the keyboard shortcut share one dialog instance.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (!isCheatsheetShortcut(e)) return
      e.preventDefault()
      setCheatsheetOpen(true)
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [])
  const overwriteSnapshotRef = useRef<Plate[]>([])
  const openChat = useChatUI((s) => s.setOpen)
  const openChatWith = useChatUI((s) => s.openWith)
  const setChatMode = useChatUI((s) => s.setMode)

  const settingsQuery = useSettings()
  const settingValue = (key: string, fallback: string) =>
    settingsQuery.data?.items.find((i) => i.key === key)?.value ?? fallback

  const anchorMode = settingValue("plan.anchor", "today") as AnchorMode
  const shoppingDay = Number(settingValue("plan.shopping_day", "1"))
  const weekStartsOn = settingValue("plan.week_starts_on", "monday") as
    | "monday"
    | "sunday"
    | "saturday"

  // Derive anchor fresh each render (pure derivation — no state)
  const anchor = computeAnchor({ mode: anchorMode, shoppingDay, weekStartsOn })
  const shifted = new Date(anchor)
  shifted.setDate(anchor.getDate() + windowOffset)
  const { from, to } = windowRange(shifted, 7)

  const slotsQuery = useTimeSlots(true)
  const platesQuery = usePlatesRange(from, to)
  const { data: aiSettings } = useAISettings()

  const plates = useMemo(
    () => platesQuery.data?.plates ?? [],
    [platesQuery.data]
  )

  const days: PlannerDay[] = useMemo(() => {
    const result: PlannerDay[] = []
    const start = new Date(from + "T00:00:00")
    for (let i = 0; i < 7; i++) {
      const d = new Date(start)
      d.setDate(start.getDate() + i)
      const dateStr = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
      const weekday = (d.getDay() + 6) % 7 // 0=Mon…6=Sun
      result.push({
        date: dateStr,
        weekday,
        plates: plates.filter((p) => p.date === dateStr),
      })
    }
    return result
  }, [from, plates])

  const nutritionQuery = useNutritionRange(from, to)

  const aiFill = usePlannerUI((s) => s.aiFill)
  const recordAiFilledPlate = usePlannerUI((s) => s.recordAiFilledPlate)
  const dismissAiFillBanner = usePlannerUI((s) => s.dismissAiFillBanner)
  const reopenAiFillBanner = usePlannerUI((s) => s.reopenAiFillBanner)
  const endAiFillSession = usePlannerUI((s) => s.endAiFillSession)
  const startAiFill = usePlannerUI((s) => s.startAiFill)

  function handleAiFill() {
    startAiFill({ from, to })
    setChatMode("fill_empty")
    openChatWith(t("planner.fill_empty.progress"))
  }

  // Watch plates created after the fill session started. Zustand actions don't
  // trigger re-render loops, so calling recordAiFilledPlate inside an effect is safe.
  useEffect(() => {
    if (
      !aiFill.startedAt ||
      aiFill.range?.from !== from ||
      aiFill.range?.to !== to
    )
      return
    for (const p of plates) {
      const created = Date.parse(p.created_at)
      if (!Number.isNaN(created) && created >= aiFill.startedAt) {
        recordAiFilledPlate(p.id)
      }
    }
  }, [aiFill, plates, from, to, recordAiFilledPlate])

  async function handleRevert() {
    for (const id of aiFill.plateIds) {
      await deletePlate(id)
    }
    endAiFillSession()
    await queryClient.invalidateQueries({ queryKey: plateKeys.range(from, to) })
    void queryClient.invalidateQueries({ queryKey: ["nutrition"] })
  }

  function handleClearWindow() {
    if (!plates.length) return
    const snapshot = plates
    queryClient.setQueryData(plateKeys.range(from, to), { plates: [] })
    const timeoutId = setTimeout(async () => {
      try {
        await Promise.all(snapshot.map((p) => deletePlate(p.id)))
        void queryClient.invalidateQueries({
          queryKey: plateKeys.range(from, to),
        })
        void queryClient.invalidateQueries({ queryKey: ["nutrition"] })
      } catch (err) {
        toastError(err, t)
        void queryClient.invalidateQueries({
          queryKey: plateKeys.range(from, to),
        })
        void queryClient.invalidateQueries({ queryKey: ["nutrition"] })
      }
    }, 5000)
    toast(t("planner.week_cleared"), {
      action: {
        label: t("common.undo"),
        onClick: () => {
          clearTimeout(timeoutId)
          queryClient.setQueryData(plateKeys.range(from, to), {
            plates: snapshot,
          })
        },
      },
      duration: 5000,
    })
  }

  const slots = slotsQuery.data?.items ?? []

  const [copyingLastWeek, setCopyingLastWeek] = useState(false)

  async function handleCopyLastWeek() {
    if (copyingLastWeek) return
    const prevFrom = shiftYMD(from, -7)
    const prevTo = shiftYMD(to, -7)
    setCopyingLastWeek(true)
    try {
      const prev = await listPlates(prevFrom, prevTo)
      // Skip skipped plates and components-less plates — they wouldn't add
      // anything meaningful to the new week and the user can re-skip if needed.
      const sourcePlates = prev.plates.filter(
        (p) => !p.skipped && p.components.length > 0
      )
      if (sourcePlates.length === 0) {
        toast(t("planner.empty_week.copy_empty"))
        return
      }
      // Two-phase parallelism — create every plate at once, then add every
      // component at once. Cuts ~N×M sequential round-trips down to two
      // parallel waves; backend has no ordering constraints across plates.
      const createdPlates = await Promise.all(
        sourcePlates.map((p) =>
          createPlate({
            date: shiftYMD(p.date, 7),
            slot_id: p.slot_id,
            note: p.note ?? undefined,
          })
        )
      )
      await Promise.all(
        createdPlates.flatMap((created, idx) =>
          sourcePlates[idx]!.components.map((pc) =>
            addPlateComponent(created.id, componentToAddInput(pc))
          )
        )
      )
      void queryClient.invalidateQueries({
        queryKey: plateKeys.range(from, to),
      })
      void queryClient.invalidateQueries({ queryKey: ["nutrition"] })
      toast.success(
        t("planner.empty_week.copied", { count: sourcePlates.length })
      )
    } catch (err) {
      toastError(err, t)
    } finally {
      setCopyingLastWeek(false)
    }
  }

  // Skipped plates are not occupied — a skip marker means "I won't eat here",
  // so applying a template should fill the slot without a conflict warning.
  const occupiedSlotKeys = useMemo(() => {
    const set = new Set<string>()
    for (const p of plates) {
      if (p.skipped) continue
      set.add(`${p.date}|${p.slot_id}`)
    }
    return set
  }, [plates])

  if (slotsQuery.isLoading || platesQuery.isLoading) {
    return (
      <div className="mx-auto max-w-7xl px-4 py-8 md:px-8 md:py-12">
        <p className="text-sm text-on-surface-variant">{t("common.loading")}</p>
      </div>
    )
  }

  if (slots.length === 0) {
    return (
      <div className="mx-auto max-w-7xl px-4 py-8 md:px-8 md:py-12">
        <section className="editorial-shadow mx-auto flex max-w-2xl flex-col items-center gap-4 rounded-2xl bg-surface-container-lowest py-16 text-center">
          <Settings className="size-10 text-on-surface-variant" aria-hidden />
          <h2 className="font-heading text-2xl font-bold text-on-surface">
            {t("planner.empty_state_no_slots_title")}
          </h2>
          <p className="max-w-md text-sm text-on-surface-variant">
            {t("planner.empty_state_no_slots_body")}
          </p>
          <Button asChild>
            <Link to="/settings">{t("planner.empty_state_no_slots_cta")}</Link>
          </Button>
        </section>
      </div>
    )
  }

  const fmt = new Intl.DateTimeFormat(i18n.language, {
    month: "short",
    day: "numeric",
  })

  const showRevertBanner =
    aiFill.range?.from === from &&
    aiFill.range?.to === to &&
    !aiFill.dismissed &&
    aiFill.plateIds.length > 0

  // Pill shown after the user dismisses the revert banner — keeps the AI
  // session reachable so revert never becomes a one-shot decision.
  const showAiSessionPill =
    aiFill.range?.from === from &&
    aiFill.range?.to === to &&
    aiFill.dismissed &&
    aiFill.plateIds.length > 0

  const weekTemplateName = t("template.name_suggestion_week", {
    date: fmt.format(new Date(from + "T00:00:00")),
    defaultValue: `Week · ${fmt.format(new Date(from + "T00:00:00"))}`,
  })

  const dailyAvgKcal = (() => {
    const days_ = nutritionQuery.data?.days
    if (!days_?.length) return null
    const plannedDays = days_.filter((d) => d.macros.kcal > 0)
    if (!plannedDays.length) return null
    const avg =
      plannedDays.reduce((acc, d) => acc + d.macros.kcal, 0) /
      plannedDays.length
    return Math.round(avg)
  })()

  return (
    <div className="mx-auto max-w-7xl space-y-8 px-4 py-8 md:px-8 md:py-12">
      <PageHeader title={t("planner.title")} />

      {showRevertBanner && (
        <RevertBanner
          count={aiFill.plateIds.length}
          onRevert={handleRevert}
          onDismiss={dismissAiFillBanner}
        />
      )}

      <div
        className="flex flex-wrap items-center justify-between gap-4 rounded-2xl bg-surface-container-low/60 px-4 py-3"
        data-testid="planner-toolbar"
      >
        <DateRangeNavigator
          from={from}
          to={to}
          onPrev={() => setWindowOffset((o) => o - 7)}
          onNext={() => setWindowOffset((o) => o + 7)}
          onToday={() => setWindowOffset(0)}
        />
        <TooltipProvider>
          <div className="flex flex-wrap items-center gap-2">
            {showAiSessionPill && (
              <button
                type="button"
                onClick={reopenAiFillBanner}
                data-testid="ai-session-pill"
                aria-label={t("planner.ai_session_pill_aria", {
                  count: aiFill.plateIds.length,
                })}
                className="flex items-center gap-1.5 rounded-full border border-ai-accent/40 bg-ai-accent-bg/70 px-3 py-1 font-heading text-[11px] font-bold tracking-[0.06em] text-ai-accent-fg uppercase transition-colors hover:bg-ai-accent-bg"
              >
                <Sparkles className="h-3 w-3" aria-hidden />
                {t("planner.ai_session_pill", {
                  count: aiFill.plateIds.length,
                })}
              </button>
            )}
            <div className="flex items-baseline gap-2 rounded-full bg-surface-container-highest px-4 py-1.5">
              <span
                className={
                  "font-heading text-sm font-bold tabular-nums " +
                  (dailyAvgKcal !== null
                    ? "text-primary"
                    : "text-on-surface-variant/40")
                }
              >
                {dailyAvgKcal !== null ? dailyAvgKcal.toLocaleString() : "—"}{" "}
                kcal
              </span>
              <span className="text-[9px] font-bold tracking-widest text-on-surface-variant uppercase">
                {t("planner.daily_avg")}
              </span>
            </div>
            <DropdownMenu>
              <Tooltip>
                <TooltipTrigger asChild>
                  <DropdownMenuTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label={t("planner.more_actions")}
                      data-testid="planner-overflow"
                      className="hover:bg-primary/10 hover:text-primary"
                    >
                      <MoreHorizontal className="size-4" />
                    </Button>
                  </DropdownMenuTrigger>
                </TooltipTrigger>
                <TooltipContent side="bottom">
                  {t("planner.more_actions")}
                </TooltipContent>
              </Tooltip>
              <DropdownMenuContent align="end" className="w-60">
                <DropdownMenuItem
                  onClick={() => setApplyWeekOpen(true)}
                  data-testid="week-template-apply"
                >
                  <FileDown className="size-4" />
                  {t("template.apply_week")}
                </DropdownMenuItem>
                <DropdownMenuItem
                  onClick={() => setSaveRangeOpen(true)}
                  disabled={plates.length === 0}
                  data-testid="week-template-save"
                >
                  <BookmarkPlus className="size-4" />
                  {t("template.save_week")}
                </DropdownMenuItem>
                <DropdownMenuItem asChild>
                  <Link to="/templates" data-testid="week-template-manage">
                    <LayoutList className="size-4" />
                    {t("template.manage")}
                  </Link>
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  onClick={() => setCopyWeekOpen(true)}
                  data-testid="copy-from-week-open"
                >
                  <CalendarRange className="size-4" />
                  {t("preset.copy_week.open")}
                </DropdownMenuItem>
                <DropdownMenuItem asChild>
                  <Link to="/presets" data-testid="presets-manage">
                    <Bookmark className="size-4" />
                    {t("preset.manage")}
                  </Link>
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={() => setNutritionOpen(true)}>
                  <BarChart2 className="size-4" />
                  {t("nutrition.button")}
                </DropdownMenuItem>
                <DropdownMenuItem
                  onClick={() => setCheatsheetOpen(true)}
                  data-testid="open-cheatsheet"
                >
                  <Keyboard className="size-4" />
                  {t("planner.shortcuts.menu_item")}
                </DropdownMenuItem>
                {aiSettings?.enabled && (
                  <>
                    <DropdownMenuItem onClick={handleAiFill}>
                      <Sparkles className="size-4" />
                      {t("planner.fill_empty_cta")}
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onClick={() => openChat(true)}
                      data-testid="chat-open-button"
                    >
                      <Sparkles className="size-4" />
                      {t("chat.button")}
                    </DropdownMenuItem>
                  </>
                )}
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  onClick={handleClearWindow}
                  disabled={plates.length === 0}
                  variant="destructive"
                  data-testid="clear-week"
                >
                  <Trash2 className="size-4" />
                  {t("planner.clear_week")}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
            <Button
              onClick={() => setShoppingOpen(true)}
              className="gradient-primary editorial-shadow border-0 text-on-primary hover:opacity-90"
            >
              <Download className="mr-1.5 size-4" />
              {t("shopping.button")}
            </Button>
          </div>
        </TooltipProvider>
      </div>

      {plates.length === 0 && (
        <EmptyWeekCTA
          windowFrom={from}
          aiEnabled={!!aiSettings?.enabled}
          copying={copyingLastWeek}
          onCopyLastWeek={() => void handleCopyLastWeek()}
          onApplyTemplate={() => setApplyWeekOpen(true)}
          onAiFill={handleAiFill}
        />
      )}

      {/* Keyed by window range so React mounts a fresh subtree per window —
          gives Tailwind's `animate-in` a clean trigger for the slide+fade.
          Wrapped in motion-safe so users with reduced-motion get an instant
          render. */}
      <div
        key={`window-${from}_${to}`}
        className="motion-safe:animate-in motion-safe:duration-150 motion-safe:ease-out motion-safe:fade-in-30 motion-safe:slide-in-from-bottom-1"
        data-testid="planner-window"
      >
        <div className="-mx-2 hidden md:-mx-4 md:block">
          <PlannerGrid
            days={days}
            slots={slots}
            rangeFrom={from}
            rangeTo={to}
            nutritionDays={nutritionQuery.data?.days}
          />
        </div>
        <div className="md:hidden">
          <MobilePlannerGrid
            days={days}
            slots={slots}
            rangeFrom={from}
            rangeTo={to}
          />
        </div>
      </div>

      <ShoppingPanel
        key={from + "_" + to}
        range={{ from, to }}
        shoppingDay={shoppingDay}
        open={shoppingOpen}
        onOpenChange={setShoppingOpen}
      />

      <Sheet open={nutritionOpen} onOpenChange={setNutritionOpen}>
        <SheetContent side="right" className="flex w-full flex-col sm:max-w-sm">
          <SheetHeader>
            <SheetTitle className="flex items-center gap-2">
              <BarChart2 className="size-4" />
              {t("nutrition.title")}
            </SheetTitle>
          </SheetHeader>
          <NutritionWeekSummary from={from} to={to} />
        </SheetContent>
      </Sheet>

      <ChatPanel range={{ from, to }} />

      <SaveAsTemplateDialog
        open={saveRangeOpen}
        onOpenChange={setSaveRangeOpen}
        target={{
          scope: "week",
          from,
          to,
          plateCount: plates.filter((p) => !p.skipped).length,
        }}
        defaultName={weekTemplateName}
      />
      <CopyFromWeekDialog
        open={copyWeekOpen}
        onOpenChange={setCopyWeekOpen}
        targetStart={from}
        defaultSourceStart={
          from
            ? new Date(new Date(from).getTime() - 7 * 24 * 60 * 60 * 1000)
                .toISOString()
                .slice(0, 10)
            : from
        }
      />
      <TemplatePicker
        open={applyWeekOpen}
        onOpenChange={setApplyWeekOpen}
        scope="week"
        defaultDate={from}
        overlap={{ occupied: occupiedSlotKeys }}
        onBeforeApply={({ overwrittenKeys }) => {
          overwriteSnapshotRef.current = snapshotOverwrittenPlates(
            from,
            to,
            overwrittenKeys
          )
        }}
        onApplied={(info) => {
          showApplyToasts(info, overwriteSnapshotRef.current, from, to, t)
          overwriteSnapshotRef.current = []
        }}
      />
      <ShortcutCheatsheet
        open={cheatsheetOpen}
        onOpenChange={setCheatsheetOpen}
      />
    </div>
  )
}
