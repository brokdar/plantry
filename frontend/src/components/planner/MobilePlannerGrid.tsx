import { format, isToday, parseISO } from "date-fns"
import * as Lucide from "lucide-react"
import { useMemo, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import {
  SaveAsTemplateDialog,
  type SaveAsTemplateTarget,
} from "@/components/templates/SaveAsTemplateDialog"
import { TemplatePicker } from "@/components/templates/TemplatePicker"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { Food } from "@/lib/api/foods"
import type { TimeSlot } from "@/lib/api/slots"
import {
  addPlateComponent,
  createPlate,
  deletePlate,
  type Plate,
} from "@/lib/api/plates"
import { useClearFeedback, useRecordFeedback } from "@/lib/queries/feedback"
import { useFoods, useSetFoodFavorite } from "@/lib/queries/foods"
import { useSetPlateSkipped, useUpdatePlate } from "@/lib/queries/plates"
import { queryClient } from "@/lib/query-client"
import { plateKeys } from "@/lib/queries/keys"
import { toggleSkip } from "@/lib/planner-skip"
import { slotLabel } from "@/lib/slot-label"
import { usePlannerUI } from "@/lib/stores/planner-ui"
import {
  showApplyToasts,
  snapshotOverwrittenPlates,
} from "@/lib/template-apply-toast"
import { suggestDayName } from "@/lib/template-suggest"
import { toast, toastError } from "@/lib/toast"
import { cn } from "@/lib/utils"

import { ComponentTraySheet, type TraySlotContext } from "./ComponentTraySheet"
import { MobileSlotRow } from "./MobileSlotRow"
import type { PlannerDay } from "./PlannerGrid"
import { SlotCell } from "./SlotCell"
import { SlotSheet, type SlotSheetTarget } from "./SlotSheet"

const DAY_KEYS = [
  "planner.day_mon",
  "planner.day_tue",
  "planner.day_wed",
  "planner.day_thu",
  "planner.day_fri",
  "planner.day_sat",
  "planner.day_sun",
] as const

interface MobilePlannerGridProps {
  days: PlannerDay[]
  slots: TimeSlot[]
  rangeFrom: string
  rangeTo: string
}

function SlotIcon({ name }: { name: string }) {
  const Icon = (
    Lucide as unknown as Record<string, Lucide.LucideIcon | undefined>
  )[name]
  if (!Icon) return <Lucide.HelpCircle className="h-4 w-4" aria-hidden />
  return <Icon className="h-4 w-4" aria-hidden />
}

interface AddTarget {
  dayIdx: number
  slotId: number
}

export function MobilePlannerGrid({
  days,
  slots,
  rangeFrom,
  rangeTo,
}: MobilePlannerGridProps) {
  const { t, i18n } = useTranslation()

  const componentsQuery = useFoods({ limit: 200 })
  const componentsById = useMemo(() => {
    const map = new Map<number, Food>()
    for (const c of componentsQuery.data?.items ?? []) map.set(c.id, c)
    return map
  }, [componentsQuery.data])

  const slotsById = useMemo(() => {
    const map = new Map<number, TimeSlot>()
    for (const s of slots) map.set(s.id, s)
    return map
  }, [slots])

  const recentFoods = useMemo(() => {
    const seen = new Set<number>()
    const out: Food[] = []
    const sortedDays = [...days].sort((a, b) =>
      a.date < b.date ? 1 : a.date > b.date ? -1 : 0
    )
    for (const day of sortedDays) {
      for (const plate of day.plates) {
        for (const pc of plate.components) {
          if (seen.has(pc.food_id)) continue
          const food = componentsById.get(pc.food_id)
          if (!food) continue
          seen.add(pc.food_id)
          out.push(food)
          if (out.length >= 20) return out
        }
      }
    }
    return out
  }, [days, componentsById])

  // Default to today's index; fall back to 0 if today isn't in the window.
  const todayStr = new Date().toISOString().slice(0, 10)
  const todayIdx = days.findIndex((d) => d.date === todayStr)
  const [activeDay, setActiveDay] = useState(todayIdx >= 0 ? todayIdx : 0)
  const [addTarget, setAddTarget] = useState<AddTarget | null>(null)
  const [sheetTarget, setSheetTarget] = useState<SlotSheetTarget | null>(null)
  const [saveTarget, setSaveTarget] = useState<SaveAsTemplateTarget | null>(
    null
  )

  const [applyDayDate, setApplyDayDate] = useState<string | null>(null)
  const overwriteSnapshotRef = useRef<Plate[]>([])

  function openSaveSlot(plateId: number) {
    let plate: Plate | undefined
    for (const d of days) {
      const p = d.plates.find((p) => p.id === plateId)
      if (p) {
        plate = p
        break
      }
    }
    if (!plate) return
    setSaveTarget({
      scope: "slot",
      plateId,
      componentCount: plate.components.length,
    })
  }

  function openSaveDay(date: string) {
    const day = days.find((d) => d.date === date)
    if (!day) return
    setSaveTarget({
      scope: "day",
      date,
      plateCount: day.plates.filter((p) => !p.skipped).length,
    })
  }

  // Skipped plates are not occupied — see desktop equivalent for rationale.
  const occupiedSlotKeys = useMemo(() => {
    const set = new Set<string>()
    for (const day of days) {
      for (const p of day.plates) {
        if (p.skipped) continue
        set.add(`${day.date}|${p.slot_id}`)
      }
    }
    return set
  }, [days])

  function handleClearDay(dayIdx: number) {
    const targetDay = days[dayIdx]
    if (!targetDay || targetDay.plates.length === 0) return
    const dayPlates = targetDay.plates
    const dayPlateIds = new Set(dayPlates.map((p) => p.id))
    queryClient.setQueryData<{ plates: Plate[] }>(
      plateKeys.range(rangeFrom, rangeTo),
      (old) => ({
        plates: (old?.plates ?? []).filter((p) => !dayPlateIds.has(p.id)),
      })
    )
    const timeoutId = setTimeout(async () => {
      try {
        await Promise.all(dayPlates.map((p) => deletePlate(p.id)))
      } catch (err) {
        toastError(err, t)
      } finally {
        void queryClient.invalidateQueries({
          queryKey: plateKeys.range(rangeFrom, rangeTo),
        })
      }
    }, 5000)
    toast(t("planner.day_cleared"), {
      action: {
        label: t("common.undo"),
        onClick: () => {
          clearTimeout(timeoutId)
          queryClient.setQueryData<{ plates: Plate[] }>(
            plateKeys.range(rangeFrom, rangeTo),
            (old) => ({ plates: [...(old?.plates ?? []), ...dayPlates] })
          )
        },
      },
      duration: 5000,
    })
  }

  const setFavoriteMut = useSetFoodFavorite()
  const setSkippedMut = useSetPlateSkipped()
  const updatePlateMut = useUpdatePlate(rangeFrom, rangeTo)
  const recordFeedbackMut = useRecordFeedback()
  const clearFeedbackMut = useClearFeedback()
  const clearAiFillOnPlate = usePlannerUI((s) => s.clearAiFillOnPlate)

  // Per-cell undo for delete: optimistically remove the plate from cache,
  // surface a 5 s undo toast, and only commit the network delete after the
  // window expires. Mirrors the desktop PlannerGrid pattern so mobile users
  // get the same recovery affordance.
  type PendingDelete = {
    timeoutId: ReturnType<typeof setTimeout>
    snapshot: Plate
  }
  const pendingDeletesRef = useRef(new Map<number, PendingDelete>())

  function openSheetForPlate(dayIdx: number, slot: TimeSlot, plate: Plate) {
    const day = days[dayIdx]
    if (!day) return
    setSheetTarget({
      plateId: plate.id,
      date: day.date,
      weekday: day.weekday,
      slotId: slot.id,
      slotNameKey: slot.name_key,
    })
  }

  function handleDeletePlate(plateId: number) {
    if (pendingDeletesRef.current.has(plateId)) return
    let snapshot: Plate | undefined
    for (const d of days) {
      const p = d.plates.find((p) => p.id === plateId)
      if (p) {
        snapshot = p
        break
      }
    }
    if (!snapshot) return
    queryClient.setQueryData<{ plates: Plate[] }>(
      plateKeys.range(rangeFrom, rangeTo),
      (old) => ({
        plates: (old?.plates ?? []).filter((p) => p.id !== plateId),
      })
    )
    const timeoutId = setTimeout(async () => {
      pendingDeletesRef.current.delete(plateId)
      try {
        await deletePlate(plateId)
      } catch (err) {
        toastError(err, t)
        if (snapshot) {
          queryClient.setQueryData<{ plates: Plate[] }>(
            plateKeys.range(rangeFrom, rangeTo),
            (old) => ({ plates: [...(old?.plates ?? []), snapshot!] })
          )
        }
      } finally {
        void queryClient.invalidateQueries({
          queryKey: plateKeys.range(rangeFrom, rangeTo),
        })
      }
    }, 5000)
    pendingDeletesRef.current.set(plateId, { timeoutId, snapshot })
    toast(t("plate.deleted"), {
      action: {
        label: t("common.undo"),
        onClick: () => {
          const pending = pendingDeletesRef.current.get(plateId)
          if (!pending) return
          clearTimeout(pending.timeoutId)
          pendingDeletesRef.current.delete(plateId)
          queryClient.setQueryData<{ plates: Plate[] }>(
            plateKeys.range(rangeFrom, rangeTo),
            (old) => ({ plates: [...(old?.plates ?? []), pending.snapshot] })
          )
        },
      },
      duration: 5000,
    })
  }

  async function handleRate(
    plateId: number,
    status: "loved" | "disliked",
    current?: string
  ) {
    try {
      if (current === status) {
        await clearFeedbackMut.mutateAsync(plateId)
      } else {
        await recordFeedbackMut.mutateAsync({ plateId, input: { status } })
      }
    } catch (err) {
      toastError(err, t)
    }
  }

  const openPicker = (dayIdx: number, slotId: number) => {
    setAddTarget({ dayIdx, slotId })
  }

  async function handleTrayCommit(
    items: { food_id: number; portions: number }[]
  ): Promise<{ failedFoodIds: number[] }> {
    if (!addTarget || items.length === 0) return { failedFoodIds: [] }
    const target = addTarget
    const targetDay = days[target.dayIdx]
    if (!targetDay) return { failedFoodIds: [] }

    let plateId: number
    try {
      const created = await createPlate({
        date: targetDay.date,
        slot_id: target.slotId,
      })
      plateId = created.id
    } catch (err) {
      toastError(err, t)
      return { failedFoodIds: items.map((i) => i.food_id) }
    }

    const results = await Promise.allSettled(
      items.map((it) =>
        addPlateComponent(plateId, {
          food_id: it.food_id,
          portions: it.portions,
        })
      )
    )
    const failedFoodIds: number[] = []
    let firstError: unknown
    results.forEach((r, idx) => {
      if (r.status === "rejected") {
        failedFoodIds.push(items[idx]!.food_id)
        firstError ??= r.reason
      }
    })
    void queryClient.invalidateQueries({
      queryKey: plateKeys.range(rangeFrom, rangeTo),
    })

    const ok = items.length - failedFoodIds.length
    if (failedFoodIds.length === 0) {
      toast(
        ok === 1
          ? t("tray.committed_one")
          : t("tray.committed_other", { count: ok })
      )
    } else if (ok > 0) {
      toast(
        failedFoodIds.length === 1
          ? t("tray.partial_failure_one")
          : t("tray.partial_failure_other", { count: failedFoodIds.length })
      )
    } else {
      toastError(firstError, t)
    }
    return { failedFoodIds }
  }

  function buildTrayContext(target: AddTarget): TraySlotContext | null {
    const day = days[target.dayIdx]
    const slot = slotsById.get(target.slotId)
    if (!day || !slot) return null
    return {
      slotId: slot.id,
      slotNameKey: slot.name_key,
      date: day.date,
      weekday: day.weekday,
    }
  }

  async function handleToggleSkip(
    dayIdx: number,
    slotId: number,
    _plateId: number | null,
    noteOverride?: string | null
  ) {
    const targetDay = days[dayIdx]
    if (!targetDay) return
    try {
      await toggleSkip({
        date: targetDay.date,
        slotId,
        existing: targetDay.plates.find((p) => p.slot_id === slotId),
        noteOverride,
        rangeFrom,
        rangeTo,
        setSkipped: setSkippedMut.mutateAsync,
      })
    } catch (err) {
      toastError(err, t)
    }
  }

  async function handleToggleFavorite(
    componentId: number | undefined,
    current: boolean
  ) {
    if (!componentId) return
    try {
      await setFavoriteMut.mutateAsync({ id: componentId, favorite: !current })
    } catch (err) {
      toastError(err, t)
    }
  }

  // Which row's swipe drawer is currently revealed (slot id, scoped to the
  // active day). Only one drawer at a time — opening row B closes row A so
  // the screen stays calm.
  const [drawerSlotId, setDrawerSlotId] = useState<number | null>(null)
  // Long-press drag bookkeeping. We hit-test the day tabs on every move so
  // the user gets immediate visual confirmation of where the drop will land.
  const [draggingPlateId, setDraggingPlateId] = useState<number | null>(null)
  const [hoveredDayIdx, setHoveredDayIdx] = useState<number | null>(null)

  function hitTestDayTab(clientX: number, clientY: number): number | null {
    const els = document.elementsFromPoint(clientX, clientY)
    for (const el of els) {
      const v = (el as HTMLElement).dataset?.mobileDayDrop
      if (v != null) return Number(v)
    }
    return null
  }

  async function handleDropOnDay(plateId: number, targetDayIdx: number) {
    const targetDay = days[targetDayIdx]
    if (!targetDay) return
    let plate: Plate | undefined
    for (const d of days) {
      const p = d.plates.find((p) => p.id === plateId)
      if (p) {
        plate = p
        break
      }
    }
    if (!plate) return
    if (plate.date === targetDay.date) return
    try {
      await updatePlateMut.mutateAsync({
        id: plateId,
        input: { date: targetDay.date, slot_id: plate.slot_id },
      })
      const dayKey = DAY_KEYS[targetDay.weekday] ?? DAY_KEYS[0]
      toast(t("planner.mobile.moved_to", { day: t(dayKey) }))
      // Follow the plate so the user sees it land.
      setActiveDay(targetDayIdx)
    } catch (err) {
      toastError(err, t, t("planner.mobile.move_failed"))
    }
  }

  // Switching active day or starting a drag should snap any open drawer shut.
  function changeActiveDay(idx: number) {
    setDrawerSlotId(null)
    setActiveDay(idx)
  }

  const activeData = days[activeDay]

  return (
    <div className="flex flex-col gap-4">
      <div
        className="grid gap-1 rounded-2xl bg-surface-container-low p-2"
        style={{
          gridTemplateColumns: `repeat(${days.length}, minmax(0, 1fr))`,
        }}
        role="tablist"
        aria-label={t("planner.title")}
      >
        {days.map((day, idx) => {
          const date = parseISO(day.date)
          const active = idx === activeDay
          const dayIsToday = isToday(date)
          const dayKey = DAY_KEYS[day.weekday] ?? DAY_KEYS[idx % 7]
          const isDropZone = draggingPlateId !== null
          const isHovered = isDropZone && hoveredDayIdx === idx
          return (
            <button
              key={day.date}
              type="button"
              role="tab"
              aria-selected={active}
              onClick={() => changeActiveDay(idx)}
              data-testid={`mobile-day-tab-${idx}`}
              data-mobile-day-drop={idx}
              data-drop-active={isDropZone ? "true" : undefined}
              data-drop-hovered={isHovered ? "true" : undefined}
              className={cn(
                "relative flex flex-col items-center gap-0.5 rounded-xl py-2 transition-[transform,background-color,box-shadow] duration-150",
                active && "bg-surface-container-high",
                isDropZone && "ring-1 ring-primary/30",
                isHovered &&
                  "scale-[1.06] bg-primary/15 text-primary shadow-[0_4px_12px_-2px_rgba(74,101,77,0.35)] ring-2 ring-primary"
              )}
            >
              <span
                className={cn(
                  "font-heading text-[10px] font-bold tracking-[0.1em] uppercase",
                  active ? "text-primary" : "text-on-surface-variant",
                  isHovered && "text-primary"
                )}
              >
                {t(dayKey)}
              </span>
              <span
                className={cn(
                  "font-heading text-[15px] font-bold",
                  dayIsToday && !active && "text-primary"
                )}
              >
                {format(date, "d")}
              </span>
            </button>
          )
        })}
      </div>

      {activeData && (
        <DayActionsBar
          dayDate={activeData.date}
          weekday={activeData.weekday}
          hasPlates={activeData.plates.filter((p) => !p.skipped).length > 0}
          onSaveDay={() => openSaveDay(activeData.date)}
          onApplyDay={() => setApplyDayDate(activeData.date)}
          onClearDay={() => handleClearDay(activeDay)}
        />
      )}

      <ul className="flex flex-col gap-3">
        {slots.map((slot) => {
          const plate = activeData?.plates.find((p) => p.slot_id === slot.id)
          const planned = !!plate && !plate.skipped
          const dayKey = activeData
            ? (DAY_KEYS[activeData.weekday] ?? DAY_KEYS[0])
            : ""
          const dragLabel = `${dayKey ? t(dayKey) : ""} · ${slotLabel(
            t,
            slot.name_key
          )}`
          return (
            <li
              key={slot.id}
              data-testid={`mobile-cell-${activeDay}-${slot.id}`}
            >
              <div className="mb-1.5 flex items-center gap-2 px-1">
                <SlotIcon name={slot.icon} />
                <span className="font-heading text-[11px] font-bold tracking-[0.16em] text-on-surface-variant uppercase">
                  {slotLabel(t, slot.name_key)}
                </span>
              </div>
              <MobileSlotRow
                planned={planned}
                drawerOpen={planned && drawerSlotId === slot.id}
                setDrawerOpen={(open) => {
                  setDrawerSlotId(open ? slot.id : null)
                }}
                onSkip={() => {
                  void handleToggleSkip(activeDay, slot.id, plate?.id ?? null)
                  if (plate) clearAiFillOnPlate(plate.id)
                }}
                onSave={plate ? () => openSaveSlot(plate.id) : undefined}
                onDelete={() => plate && handleDeletePlate(plate.id)}
                onDragStart={() => {
                  if (!plate) return
                  setDrawerSlotId(null)
                  setDraggingPlateId(plate.id)
                  setHoveredDayIdx(null)
                }}
                onDragMove={(x, y) => {
                  setHoveredDayIdx(hitTestDayTab(x, y))
                }}
                onDragEnd={(x, y) => {
                  const target = hitTestDayTab(x, y)
                  setDraggingPlateId(null)
                  setHoveredDayIdx(null)
                  if (plate && target !== null && target !== activeDay) {
                    void handleDropOnDay(plate.id, target)
                  }
                }}
                dragLabel={dragLabel}
                testId={`mobile-slot-row-${slot.id}`}
              >
                <SlotCell
                  day={activeDay}
                  slotId={slot.id}
                  plate={plate}
                  componentsById={componentsById}
                  onAdd={() => openPicker(activeDay, slot.id)}
                  onOpenSheet={
                    plate
                      ? () => openSheetForPlate(activeDay, slot, plate)
                      : undefined
                  }
                  onDeletePlate={() => plate && handleDeletePlate(plate.id)}
                  onToggleFavorite={() => {
                    const hero = plate?.components
                      .slice()
                      .sort((a, b) => a.sort_order - b.sort_order)[0]
                    const heroComp = hero
                      ? componentsById.get(hero.food_id)
                      : undefined
                    void handleToggleFavorite(
                      heroComp?.id,
                      heroComp?.favorite ?? false
                    )
                    if (plate) clearAiFillOnPlate(plate.id)
                  }}
                  onToggleSkip={(note) => {
                    void handleToggleSkip(
                      activeDay,
                      slot.id,
                      plate?.id ?? null,
                      note
                    )
                    if (plate) clearAiFillOnPlate(plate.id)
                  }}
                  onRateLoved={() => {
                    if (!plate) return
                    void handleRate(plate.id, "loved", plate.feedback?.status)
                    clearAiFillOnPlate(plate.id)
                  }}
                  onRateDisliked={() => {
                    if (!plate) return
                    void handleRate(
                      plate.id,
                      "disliked",
                      plate.feedback?.status
                    )
                    clearAiFillOnPlate(plate.id)
                  }}
                />
              </MobileSlotRow>
            </li>
          )
        })}
      </ul>

      <ComponentTraySheet
        open={addTarget !== null}
        context={addTarget ? buildTrayContext(addTarget) : null}
        recentFoods={recentFoods}
        side="bottom"
        onOpenChange={(o) => !o && setAddTarget(null)}
        onCommit={(items) => handleTrayCommit(items)}
      />
      <SaveAsTemplateDialog
        open={saveTarget !== null}
        onOpenChange={(o) => !o && setSaveTarget(null)}
        target={saveTarget}
        defaultName={
          saveTarget?.scope === "day"
            ? suggestDayName(
                t,
                i18n.language,
                saveTarget.date,
                days.find((d) => d.date === saveTarget.date)?.weekday ?? 0
              )
            : ""
        }
      />
      <TemplatePicker
        open={applyDayDate !== null}
        onOpenChange={(o) => !o && setApplyDayDate(null)}
        scope="day"
        defaultDate={applyDayDate ?? rangeFrom}
        overlap={{ occupied: occupiedSlotKeys }}
        onBeforeApply={({ overwrittenKeys }) => {
          overwriteSnapshotRef.current = snapshotOverwrittenPlates(
            rangeFrom,
            rangeTo,
            overwrittenKeys
          )
        }}
        onApplied={(info) => {
          showApplyToasts(
            info,
            overwriteSnapshotRef.current,
            rangeFrom,
            rangeTo,
            t
          )
          overwriteSnapshotRef.current = []
        }}
      />
      <SlotSheet
        target={sheetTarget}
        days={days}
        componentsById={componentsById}
        rangeFrom={rangeFrom}
        rangeTo={rangeTo}
        side="bottom"
        onOpenChange={(open) => {
          if (!open) setSheetTarget(null)
        }}
        onAddComponent={(target) =>
          setAddTarget({
            dayIdx: days.findIndex((d) => d.date === target.date),
            slotId: target.slotId,
          })
        }
        onSwapComponent={() => {
          // Swap is opened via the same picker; mobile picker handles 'add only'.
          // Phase 5 will replace this with a dedicated swap flow.
          if (sheetTarget) {
            setAddTarget({
              dayIdx: days.findIndex((d) => d.date === sheetTarget.date),
              slotId: sheetTarget.slotId,
            })
          }
        }}
        onSaveAsTemplate={(plateId) => openSaveSlot(plateId)}
        onToggleSkip={(target, currentSkipped) => {
          const dayIdx = days.findIndex((d) => d.date === target.date)
          if (dayIdx < 0) return
          void handleToggleSkip(dayIdx, target.slotId, target.plateId)
          if (!currentSkipped) setSheetTarget(null)
        }}
        onDeletePlate={(plateId) => {
          handleDeletePlate(plateId)
          setSheetTarget(null)
        }}
        onMovePlate={(target, newDate) => {
          const targetIdx = days.findIndex((d) => d.date === newDate)
          if (targetIdx < 0 || newDate === target.date) return
          setSheetTarget(null)
          void handleDropOnDay(target.plateId, targetIdx)
        }}
      />
    </div>
  )
}

const DAY_ACTION_KEYS = DAY_KEYS

/** Mobile equivalent of the desktop DayHeader overflow — apply-day,
 * save-day, clear-day. Sits above the active day's slot list so a phone
 * user has feature parity with the desktop column header. */
function DayActionsBar({
  dayDate,
  weekday,
  hasPlates,
  onSaveDay,
  onApplyDay,
  onClearDay,
}: {
  dayDate: string
  weekday: number
  hasPlates: boolean
  onSaveDay: () => void
  onApplyDay: () => void
  onClearDay: () => void
}) {
  const { t, i18n } = useTranslation()
  const dayKey = DAY_ACTION_KEYS[weekday] ?? DAY_ACTION_KEYS[0]
  const date = parseISO(dayDate)
  const dateLabel = new Intl.DateTimeFormat(i18n.language, {
    month: "short",
    day: "numeric",
  }).format(date)
  return (
    <div className="-mt-1 flex items-center justify-between gap-2 px-1">
      <span className="font-heading text-[10.5px] font-bold tracking-[0.18em] text-on-surface-variant uppercase">
        {t(dayKey)} · {dateLabel}
      </span>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t("planner.day_actions_for", {
              day: t(dayKey),
              date: dateLabel,
            })}
            data-testid="mobile-day-menu"
            className="size-7 text-on-surface-variant"
          >
            <Lucide.MoreHorizontal className="h-4 w-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-56">
          <DropdownMenuItem onClick={onApplyDay} data-testid="mobile-day-apply">
            <Lucide.FileDown className="size-4" />
            {t("template.apply_day")}
          </DropdownMenuItem>
          <DropdownMenuItem
            onClick={onSaveDay}
            disabled={!hasPlates}
            data-testid="mobile-day-save"
          >
            <Lucide.BookmarkPlus className="size-4" />
            {t("template.save_day")}
          </DropdownMenuItem>
          {hasPlates && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                variant="destructive"
                onClick={onClearDay}
                data-testid="mobile-day-clear"
              >
                <Lucide.Trash2 className="size-4" />
                {t("planner.clear_day")}
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
