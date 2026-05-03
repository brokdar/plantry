import { format, parseISO } from "date-fns"
import { de as deLocale } from "date-fns/locale"
import {
  ArrowLeftRight,
  BookmarkPlus,
  Heart,
  Minus,
  Plus,
  Sparkles,
  StickyNote,
  ThumbsDown,
  ThumbsUp,
  Trash2,
  Utensils,
  X,
} from "lucide-react"
import { useId, useMemo } from "react"
import { useTranslation } from "react-i18next"

import {
  FoodPlaceholder,
  type FoodPlaceholderCategory,
} from "@/components/editorial/FoodPlaceholder"
import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Textarea } from "@/components/ui/textarea"
import type { Food } from "@/lib/api/foods"
import type { Plate, PlateComponent } from "@/lib/api/plates"
import { useClearFeedback, useRecordFeedback } from "@/lib/queries/feedback"
import {
  useRemovePlateComponent,
  useUpdatePlate,
  useUpdatePlateComponentPortions,
} from "@/lib/queries/plates"
import { imageURL } from "@/lib/image-url"
import { slotLabel } from "@/lib/slot-label"
import { toastError } from "@/lib/toast"
import { cn } from "@/lib/utils"

import type { PlannerDay } from "./PlannerGrid"

const DAY_KEYS = [
  "planner.day_mon",
  "planner.day_tue",
  "planner.day_wed",
  "planner.day_thu",
  "planner.day_fri",
  "planner.day_sat",
  "planner.day_sun",
] as const

interface SlotSheetTarget {
  plateId: number
  date: string
  weekday: number
  slotId: number
  slotNameKey: string
}

interface SlotSheetProps {
  target: SlotSheetTarget | null
  /** Sourced from the latest planner cache so the sheet reflects optimistic edits without re-fetching. */
  days: PlannerDay[]
  componentsById: Map<number, Food>
  rangeFrom: string
  rangeTo: string
  aiFilled?: boolean
  side?: "right" | "bottom"
  onOpenChange: (open: boolean) => void
  onAddComponent: (target: SlotSheetTarget) => void
  onSwapComponent: (
    target: SlotSheetTarget,
    pcId: number,
    defaultRole?: string
  ) => void
  onSaveAsTemplate: (plateId: number) => void
  onToggleSkip: (target: SlotSheetTarget, currentSkipped: boolean) => void
  onDeletePlate: (plateId: number) => void
  /** Move the plate to another day in the visible window. Renders the
   *  Move-to-day picker when present — keyboard / screen-reader path that
   *  parallels the desktop drag and the mobile long-press gesture. */
  onMovePlate?: (target: SlotSheetTarget, newDate: string) => void
}

export function SlotSheet({
  target,
  days,
  componentsById,
  rangeFrom,
  rangeTo,
  aiFilled,
  side = "right",
  onOpenChange,
  onAddComponent,
  onSwapComponent,
  onSaveAsTemplate,
  onToggleSkip,
  onDeletePlate,
  onMovePlate,
}: SlotSheetProps) {
  const { t } = useTranslation()

  // Look up the live plate from the freshest planner snapshot every render.
  // The parent's cache invalidates on every mutation; this keeps the sheet
  // in lockstep without holding its own copy.
  const plate = useMemo<Plate | undefined>(() => {
    if (!target) return undefined
    const day = days.find((d) => d.date === target.date)
    return day?.plates.find((p) => p.id === target.plateId)
  }, [target, days])

  const isOpen = target !== null
  const sheetSide = side === "bottom" ? "bottom" : "right"

  return (
    <Sheet open={isOpen} onOpenChange={onOpenChange}>
      <SheetContent
        side={sheetSide}
        showCloseButton={false}
        className={cn(
          "flex flex-col gap-0 border-outline-variant/40 bg-surface-container-lowest p-0",
          sheetSide === "right" &&
            "w-full sm:max-w-[440px] data-[side=right]:sm:max-w-[440px]",
          sheetSide === "bottom" && "max-h-[92dvh] rounded-t-3xl"
        )}
        data-testid="slot-sheet"
      >
        {target && plate ? (
          <SlotSheetBody
            target={target}
            plate={plate}
            componentsById={componentsById}
            days={days}
            rangeFrom={rangeFrom}
            rangeTo={rangeTo}
            aiFilled={!!aiFilled}
            onClose={() => onOpenChange(false)}
            onAddComponent={() => onAddComponent(target)}
            onSwapComponent={(pcId, role) =>
              onSwapComponent(target, pcId, role)
            }
            onSaveAsTemplate={() => onSaveAsTemplate(target.plateId)}
            onToggleSkip={() => onToggleSkip(target, plate.skipped)}
            onDeletePlate={() => onDeletePlate(target.plateId)}
            onMovePlate={
              onMovePlate
                ? (newDate) => onMovePlate(target, newDate)
                : undefined
            }
          />
        ) : (
          <SheetHeader>
            <SheetTitle className="sr-only">{t("slot_sheet.title")}</SheetTitle>
            <SheetDescription className="sr-only">
              {t("slot_sheet.empty_description")}
            </SheetDescription>
          </SheetHeader>
        )}
      </SheetContent>
    </Sheet>
  )
}

interface SlotSheetBodyProps {
  target: SlotSheetTarget
  plate: Plate
  componentsById: Map<number, Food>
  days: PlannerDay[]
  rangeFrom: string
  rangeTo: string
  aiFilled: boolean
  onClose: () => void
  onAddComponent: () => void
  onSwapComponent: (pcId: number, defaultRole?: string) => void
  onSaveAsTemplate: () => void
  onToggleSkip: () => void
  onDeletePlate: () => void
  onMovePlate?: (newDate: string) => void
}

function SlotSheetBody({
  target,
  plate,
  componentsById,
  days,
  rangeFrom,
  rangeTo,
  aiFilled,
  onClose,
  onAddComponent,
  onSwapComponent,
  onSaveAsTemplate,
  onToggleSkip,
  onDeletePlate,
  onMovePlate,
}: SlotSheetBodyProps) {
  const { t, i18n } = useTranslation()

  const slotName = slotLabel(t, target.slotNameKey)
  const dayKey = DAY_KEYS[target.weekday] ?? DAY_KEYS[0]
  const dayLabel = t(dayKey)
  const dateLocale = i18n.language?.startsWith("de") ? deLocale : undefined
  const dateLabel = format(parseISO(target.date), "MMM d", {
    locale: dateLocale,
  })

  const sorted = useMemo(
    () => [...plate.components].sort((a, b) => a.sort_order - b.sort_order),
    [plate.components]
  )
  const heroPc = sorted[0]
  const heroFood = heroPc ? componentsById.get(heroPc.food_id) : undefined

  return (
    <>
      <SheetHeader className="gap-2 border-b border-outline-variant/40 bg-surface-container-low/40 px-5 py-4">
        <div className="flex items-start justify-between gap-3">
          <div className="flex flex-col gap-1">
            <span
              className="font-heading text-[10.5px] font-bold tracking-[0.22em] text-on-surface-variant uppercase"
              data-testid="slot-sheet-eyebrow"
            >
              {dayLabel} · {dateLabel}
            </span>
            <SheetTitle className="font-heading text-[22px] leading-tight font-bold tracking-tight text-on-surface">
              {slotName}
            </SheetTitle>
            <SheetDescription className="text-[12.5px] text-on-surface-variant">
              {t("slot_sheet.description", { count: sorted.length })}
            </SheetDescription>
          </div>
          <div className="flex items-center gap-1">
            {aiFilled && (
              <span
                className="grid size-7 place-items-center rounded-full bg-[#c2974a]/12 text-[#9a7634]"
                title={t("planner.slot.ai_filled")}
                aria-label={t("planner.slot.ai_filled")}
              >
                <Sparkles className="h-3.5 w-3.5" aria-hidden />
              </span>
            )}
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              onClick={onClose}
              aria-label={t("common.close")}
              className="text-on-surface-variant"
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </SheetHeader>

      <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-5 py-5">
        {heroPc && (
          <HeroBlock
            heroFood={heroFood}
            heroPc={heroPc}
            plate={plate}
            aiFilled={aiFilled}
          />
        )}

        <ComponentList
          plate={plate}
          sorted={sorted}
          componentsById={componentsById}
          onAdd={onAddComponent}
          onSwap={onSwapComponent}
          onLastRemoved={onDeletePlate}
        />

        <NoteField plate={plate} rangeFrom={rangeFrom} rangeTo={rangeTo} />

        <FeedbackBlock plate={plate} />

        {onMovePlate && !plate.skipped && (
          <MoveToDayPicker
            days={days}
            currentDate={target.date}
            onPick={onMovePlate}
          />
        )}
      </div>

      <ActionFooter
        skipped={plate.skipped}
        onSaveAsTemplate={onSaveAsTemplate}
        onToggleSkip={onToggleSkip}
        onDeletePlate={onDeletePlate}
      />
    </>
  )
}

function HeroBlock({
  heroFood,
  heroPc,
  plate,
  aiFilled,
}: {
  heroFood: Food | undefined
  heroPc: PlateComponent
  plate: Plate
  aiFilled: boolean
}) {
  const { t } = useTranslation()
  const heroRole =
    heroFood?.kind === "composed" ? (heroFood.role ?? null) : null
  const status: { label: string; tone: string } | null = (() => {
    if (plate.feedback?.status === "loved")
      return {
        label: t("plate.feedback.loved"),
        tone: "bg-primary/12 text-primary",
      }
    if (plate.feedback?.status === "disliked")
      return {
        label: t("plate.feedback.disliked"),
        tone: "bg-destructive/10 text-destructive",
      }
    if (plate.feedback?.status === "cooked")
      return {
        label: t("plate.feedback.cooked"),
        tone: "bg-tertiary/12 text-tertiary",
      }
    return null
  })()

  return (
    <div className="relative h-44 overflow-hidden rounded-2xl border border-outline-variant/40 bg-surface-container">
      {heroFood?.image_path ? (
        <img
          src={imageURL(heroFood.image_path)}
          alt=""
          className="h-full w-full object-cover"
        />
      ) : (
        <FoodPlaceholder
          category={(heroRole ?? "main") as FoodPlaceholderCategory}
          size="lg"
          rounded="none"
          className="h-full w-full"
        />
      )}
      <div
        className="absolute inset-0 bg-gradient-to-t from-black/55 via-black/10 to-transparent"
        aria-hidden
      />
      <div className="absolute inset-x-0 bottom-0 flex flex-wrap items-end justify-between gap-2 px-4 pt-10 pb-4">
        <span className="font-heading text-[15.5px] leading-tight font-bold tracking-tight text-white drop-shadow-[0_2px_6px_rgba(0,0,0,0.35)]">
          {heroFood?.name ?? `#${heroPc.food_id}`}
        </span>
        <div className="flex items-center gap-1.5">
          {aiFilled && (
            <span className="rounded-full bg-[#c2974a] px-2 py-0.5 font-heading text-[9px] font-bold tracking-[0.2em] text-white uppercase">
              {t("planner.slot.ai_filled")}
            </span>
          )}
          {status && (
            <span
              className={cn(
                "rounded-full px-2 py-0.5 font-heading text-[9px] font-bold tracking-[0.2em] uppercase backdrop-blur",
                status.tone
              )}
            >
              {status.label}
            </span>
          )}
        </div>
      </div>
    </div>
  )
}

function ComponentList({
  plate,
  sorted,
  componentsById,
  onAdd,
  onSwap,
  onLastRemoved,
}: {
  plate: Plate
  sorted: PlateComponent[]
  componentsById: Map<number, Food>
  onAdd: () => void
  onSwap: (pcId: number, defaultRole?: string) => void
  onLastRemoved: () => void
}) {
  const { t } = useTranslation()
  return (
    <section
      aria-label={t("slot_sheet.components_label")}
      className="flex flex-col gap-2"
    >
      <div className="flex items-baseline justify-between">
        <h3 className="font-heading text-[10.5px] font-bold tracking-[0.22em] text-on-surface-variant uppercase">
          {t("slot_sheet.components_label")}
          <span className="ml-1.5 font-mono text-[11px] tracking-normal text-on-surface-variant/70 normal-case">
            ({sorted.length})
          </span>
        </h3>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={onAdd}
          data-testid="slot-sheet-add-component"
          className="-mr-2 h-7 gap-1 text-primary hover:text-primary"
        >
          <Plus className="h-3.5 w-3.5" />
          {t("plate.add_component")}
        </Button>
      </div>
      {sorted.length === 0 ? (
        <button
          type="button"
          onClick={onAdd}
          className="flex h-20 items-center justify-center rounded-2xl border border-dashed border-outline-variant/50 bg-surface-container-low/40 text-on-surface-variant transition-colors hover:border-primary/50 hover:text-on-surface"
        >
          <span className="font-heading text-[11px] font-bold tracking-[0.2em] uppercase">
            {t("slot_sheet.empty_components")}
          </span>
        </button>
      ) : (
        <ul className="flex flex-col gap-1.5">
          {sorted.map((pc) => (
            <ComponentRow
              key={pc.id}
              plate={plate}
              pc={pc}
              food={componentsById.get(pc.food_id)}
              isLast={sorted.length === 1}
              onSwap={(role) => onSwap(pc.id, role)}
              onLastRemoved={onLastRemoved}
            />
          ))}
        </ul>
      )}
    </section>
  )
}

function ComponentRow({
  plate,
  pc,
  food,
  isLast,
  onSwap,
  onLastRemoved,
}: {
  plate: Plate
  pc: PlateComponent
  food: Food | undefined
  isLast: boolean
  onSwap: (role?: string) => void
  onLastRemoved: () => void
}) {
  const { t } = useTranslation()
  const updatePortions = useUpdatePlateComponentPortions()
  const removeComponent = useRemovePlateComponent()

  const role = food?.kind === "composed" ? (food.role ?? undefined) : undefined
  const roleLabel =
    food?.kind === "composed" && food.role
      ? t(`planner.slot.role.${food.role}`, {
          defaultValue: food.role,
        })
      : null

  function commit(next: number) {
    const clamped = Math.max(0.25, Math.round(next * 4) / 4)
    if (clamped === pc.portions) return
    updatePortions
      .mutateAsync({ plateId: plate.id, pcId: pc.id, portions: clamped })
      .catch((err) => toastError(err, t))
  }

  function handleRemove() {
    // If this was the only component, removing it would leave an orphan
    // plate. Skip the per-component delete and let the parent delete the
    // whole plate instead — which also closes the sheet and offers undo.
    if (isLast) {
      onLastRemoved()
      return
    }
    removeComponent
      .mutateAsync({ plateId: plate.id, pcId: pc.id })
      .catch((err) => toastError(err, t))
  }

  return (
    <li
      className="group flex items-center gap-2 rounded-xl border border-transparent bg-surface-container-low/50 px-2 py-1.5 transition-colors hover:border-outline-variant/40 hover:bg-surface-container-low"
      data-testid={`slot-sheet-row-${pc.id}`}
    >
      <span className="grid size-9 shrink-0 place-items-center overflow-hidden rounded-lg bg-surface-container">
        {food?.image_path ? (
          <img
            src={imageURL(food.image_path)}
            alt=""
            className="h-full w-full object-cover"
          />
        ) : (
          <FoodPlaceholder
            category={(role ?? "main") as FoodPlaceholderCategory}
            size="sm"
            rounded="lg"
            className="h-full w-full"
          />
        )}
      </span>
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-[13px] font-medium text-on-surface">
          {food?.name ?? `#${pc.food_id}`}
        </span>
        {roleLabel && (
          <span className="font-heading text-[9px] font-bold tracking-[0.18em] text-on-surface-variant uppercase">
            {roleLabel}
          </span>
        )}
      </div>
      <PortionStepper value={pc.portions} onChange={commit} />
      {/* Touch devices have no hover; force the row actions visible there.
        On hover-capable devices, fade in on hover/focus to keep the row tidy. */}
      <div className="flex items-center gap-0.5 opacity-100 transition-opacity [@media(hover:hover)]:opacity-0 [@media(hover:hover)]:group-focus-within:opacity-100 [@media(hover:hover)]:group-hover:opacity-100">
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={t("plate.swap_component")}
          onClick={() => onSwap(role)}
          data-testid={`slot-sheet-swap-${pc.id}`}
          className="text-on-surface-variant hover:text-on-surface"
        >
          <ArrowLeftRight className="h-3.5 w-3.5" />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={t("plate.remove_component")}
          onClick={handleRemove}
          data-testid={`slot-sheet-remove-${pc.id}`}
          className="text-on-surface-variant hover:text-destructive"
        >
          <X className="h-3.5 w-3.5" />
        </Button>
      </div>
    </li>
  )
}

function PortionStepper({
  value,
  onChange,
}: {
  value: number
  onChange: (next: number) => void
}) {
  const { t } = useTranslation()
  return (
    <div
      className="flex items-center gap-0 rounded-full border border-outline-variant/50 bg-surface-container-lowest"
      role="group"
      aria-label={t("plate.portions")}
    >
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label={`${t("plate.portions")} −0.25`}
        onClick={() => onChange(Math.max(0.25, value - 0.25))}
        className="size-6 rounded-full text-on-surface-variant"
        disabled={value <= 0.25}
      >
        <Minus className="h-3 w-3" />
      </Button>
      <span
        className="min-w-[2.4rem] text-center font-mono text-[12px] font-semibold text-on-surface tabular-nums"
        data-testid="slot-sheet-portions-value"
      >
        ×{formatPortions(value)}
      </span>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label={`${t("plate.portions")} +0.25`}
        onClick={() => onChange(value + 0.25)}
        className="size-6 rounded-full text-on-surface-variant"
      >
        <Plus className="h-3 w-3" />
      </Button>
    </div>
  )
}

function formatPortions(n: number): string {
  if (Number.isInteger(n)) return `${n}.0`
  return n.toFixed(2).replace(/0$/, "")
}

function NoteField({
  plate,
  rangeFrom,
  rangeTo,
}: {
  plate: Plate
  rangeFrom: string
  rangeTo: string
}) {
  const { t } = useTranslation()
  const updatePlate = useUpdatePlate(rangeFrom, rangeTo)
  const id = useId()
  const initial = plate.note ?? ""

  function commit(value: string) {
    const next = value.trim()
    if (next === initial) return
    updatePlate
      .mutateAsync({
        id: plate.id,
        input: { note: next === "" ? null : next },
      })
      .catch((err) => toastError(err, t))
  }

  return (
    <section className="flex flex-col gap-1.5">
      <label
        htmlFor={id}
        className="flex items-center gap-1.5 font-heading text-[10.5px] font-bold tracking-[0.22em] text-on-surface-variant uppercase"
      >
        <StickyNote className="h-3 w-3" aria-hidden />
        {t("plate.note")}
      </label>
      <Textarea
        id={id}
        // Uncontrolled: keystrokes stay local; commit on blur. The key prop
        // remounts the field when the sheet's target changes, so opening a
        // different plate shows that plate's note instead of carrying over
        // the previous one.
        key={`${plate.id}:${initial}`}
        defaultValue={initial}
        onBlur={(e) => commit(e.target.value)}
        placeholder={t("plate.note_placeholder")}
        className="min-h-16 resize-none border-outline-variant/40 bg-surface-container-low/40 text-[13px] text-on-surface placeholder:text-on-surface-variant/60 focus-visible:border-primary/60 focus-visible:bg-surface-container-lowest"
        data-testid="slot-sheet-note"
      />
    </section>
  )
}

function FeedbackBlock({ plate }: { plate: Plate }) {
  const { t } = useTranslation()
  const record = useRecordFeedback()
  const clear = useClearFeedback()
  const current = plate.feedback?.status

  function toggle(status: "cooked" | "loved" | "disliked") {
    const action =
      current === status
        ? clear.mutateAsync(plate.id)
        : record.mutateAsync({
            plateId: plate.id,
            input: { status, note: plate.feedback?.note ?? null },
          })
    action.catch((err) => toastError(err, t))
  }

  const buttons: {
    key: "cooked" | "loved" | "disliked"
    label: string
    Icon: typeof Heart
    activeClass: string
  }[] = [
    {
      key: "cooked",
      label: t("plate.feedback.cooked"),
      Icon: Utensils,
      activeClass: "border-tertiary/60 bg-tertiary/10 text-tertiary",
    },
    {
      key: "loved",
      label: t("plate.feedback.loved"),
      Icon: ThumbsUp,
      activeClass: "border-primary/60 bg-primary/10 text-primary",
    },
    {
      key: "disliked",
      label: t("plate.feedback.disliked"),
      Icon: ThumbsDown,
      activeClass: "border-destructive/60 bg-destructive/10 text-destructive",
    },
  ]

  return (
    <section
      className="flex flex-col gap-2"
      aria-label={t("plate.feedback.label")}
      data-testid={`slot-sheet-feedback-${plate.id}`}
    >
      <h3 className="font-heading text-[10.5px] font-bold tracking-[0.22em] text-on-surface-variant uppercase">
        {t("plate.feedback.label")}
      </h3>
      <div className="grid grid-cols-3 gap-1.5">
        {buttons.map(({ key, label, Icon, activeClass }) => {
          const active = current === key
          return (
            <button
              key={key}
              type="button"
              onClick={() => toggle(key)}
              aria-pressed={active}
              aria-label={label}
              data-active={active ? "true" : undefined}
              data-testid={`slot-sheet-feedback-${key}`}
              className={cn(
                "flex flex-col items-center gap-1 rounded-xl border border-outline-variant/40 bg-surface-container-lowest px-2 py-2.5 text-on-surface-variant transition-all duration-150",
                "hover:-translate-y-px hover:border-primary/40 hover:text-on-surface",
                active && activeClass
              )}
            >
              <Icon
                className="h-4 w-4"
                fill={active ? "currentColor" : "none"}
                aria-hidden
              />
              <span className="font-heading text-[10px] font-bold tracking-[0.18em] uppercase">
                {label}
              </span>
            </button>
          )
        })}
      </div>
    </section>
  )
}

function ActionFooter({
  skipped,
  onSaveAsTemplate,
  onToggleSkip,
  onDeletePlate,
}: {
  skipped: boolean
  onSaveAsTemplate: () => void
  onToggleSkip: () => void
  onDeletePlate: () => void
}) {
  const { t } = useTranslation()
  const skipLabel = skipped ? t("skip.unmark") : t("skip.mark")
  return (
    <footer className="border-t border-outline-variant/40 bg-surface-container-low/60 px-5 py-3">
      {/* Save and Skip share the row equally and truncate their labels in
        long-string locales (German). Delete is always icon-only — the trash
        glyph is universal and avoids pushing the row past the 390 px sheet
        width on mobile. */}
      <div className="flex items-center gap-1.5">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={onSaveAsTemplate}
          data-testid="slot-sheet-save-template"
          className="h-8 min-w-0 flex-1 justify-start gap-1.5 text-on-surface-variant hover:text-on-surface"
        >
          <BookmarkPlus className="h-3.5 w-3.5 shrink-0" />
          <span className="truncate">{t("template.save_as")}</span>
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={onToggleSkip}
          data-testid="slot-sheet-skip"
          aria-label={skipLabel}
          className="h-8 min-w-0 flex-1 justify-start gap-1.5 text-on-surface-variant hover:text-on-surface"
        >
          <Utensils className="h-3.5 w-3.5 shrink-0" />
          <span className="truncate">{skipLabel}</span>
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          onClick={onDeletePlate}
          data-testid="slot-sheet-delete"
          aria-label={t("plate.delete_plate")}
          title={t("plate.delete_plate")}
          className="size-8 shrink-0 text-destructive/80 hover:bg-destructive/10 hover:text-destructive"
        >
          <Trash2 className="h-3.5 w-3.5" />
        </Button>
      </div>
    </footer>
  )
}

/**
 * Keyboard / screen-reader path for moving a plate to another day in the
 * visible window. Mirrors the desktop drag and the mobile long-press but
 * uses tappable / focusable chips so any input modality can trigger it.
 * The current day's chip is rendered as `aria-current` and disabled — moves
 * to the same day would be a no-op.
 */
function MoveToDayPicker({
  days,
  currentDate,
  onPick,
}: {
  days: PlannerDay[]
  currentDate: string
  onPick: (newDate: string) => void
}) {
  const { t, i18n } = useTranslation()
  const dateLocale = i18n.language?.startsWith("de") ? deLocale : undefined
  return (
    <section
      className="flex flex-col gap-2"
      data-testid="slot-sheet-move-picker"
      aria-label={t("planner.mobile.move_to_picker_label")}
    >
      <h3 className="font-heading text-[10.5px] font-bold tracking-[0.22em] text-on-surface-variant uppercase">
        {t("planner.mobile.move_to")}
      </h3>
      <div
        className="grid gap-1.5"
        style={{
          gridTemplateColumns: `repeat(${days.length}, minmax(0, 1fr))`,
        }}
        role="group"
      >
        {days.map((day) => {
          const isCurrent = day.date === currentDate
          const dayKey = DAY_KEYS[day.weekday] ?? DAY_KEYS[0]
          const dayShort = t(dayKey)
          const date = parseISO(day.date)
          const dayNum = format(date, "d", { locale: dateLocale })
          return (
            <button
              key={day.date}
              type="button"
              disabled={isCurrent}
              aria-current={isCurrent ? "true" : undefined}
              aria-label={
                isCurrent
                  ? `${dayShort} — ${t("planner.mobile.current_day")}`
                  : `${t("planner.mobile.move_to")}: ${dayShort}`
              }
              onClick={() => onPick(day.date)}
              data-testid={`slot-sheet-move-day-${day.weekday}`}
              className={cn(
                "flex flex-col items-center gap-0.5 rounded-xl border py-2 transition-[background-color,border-color,transform] duration-150",
                isCurrent
                  ? "cursor-default border-primary/50 bg-primary/10 text-primary"
                  : "border-outline-variant/40 bg-surface-container-low/40 text-on-surface hover:-translate-y-px hover:border-primary/40 hover:bg-surface-container-low"
              )}
            >
              <span className="font-heading text-[9px] font-bold tracking-[0.16em] uppercase">
                {dayShort}
              </span>
              <span className="font-heading text-[13px] font-bold">
                {dayNum}
              </span>
            </button>
          )
        })}
      </div>
    </section>
  )
}

export type { SlotSheetTarget }
