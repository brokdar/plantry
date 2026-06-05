import { parseISO, isBefore, isToday, startOfDay } from "date-fns"
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCorners,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core"
import { Copy, MoreHorizontal, Trash2 } from "lucide-react"
import { useMemo, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import {
  SaveAsPresetDialog,
  type SaveAsPresetTarget,
} from "@/components/presets/SaveAsPresetDialog"
import {
  dragStartToPayload,
  DndCellWrapper,
  type DragPayload,
} from "@/components/planner/DndCellWrapper"
import {
  addPlateComponent,
  componentToAddInput,
  createPlate,
  deletePlate,
} from "@/lib/api/plates"
import { handleGridArrowKey } from "@/lib/planner-keynav"
import { queryClient } from "@/lib/query-client"
import { plateKeys } from "@/lib/queries/keys"
import {
  cancelPendingPlateDelete,
  flushPendingPlateDeletes,
  hasPendingPlateDelete,
  registerPendingPlateDelete,
} from "@/lib/queries/pending-plate-deletes"
import { shoppingKeys } from "@/lib/queries/shopping"
import type { Food } from "@/lib/api/foods"
import type { MacrosResponse, Plate } from "@/lib/api/plates"
import type { TimeSlot } from "@/lib/api/slots"
import type { NutritionDay } from "@/lib/api/nutrition"
import { useFoods, useSetFoodFavorite } from "@/lib/queries/foods"
import { useClearFeedback, useRecordFeedback } from "@/lib/queries/feedback"
import {
  usePlateMacros,
  useSetPlateSkipped,
  useSwapPlateComponent,
  useUpdatePlate,
} from "@/lib/queries/plates"
import { useProfile } from "@/lib/queries/profile"
import { toggleSkip } from "@/lib/planner-skip"
import { SLOT_ICONS, SLOT_ICON_FALLBACK } from "@/lib/slot-icons"
import { slotLabel } from "@/lib/slot-label"
import { usePlannerUI } from "@/lib/stores/planner-ui"
import { toast, toastError } from "@/lib/toast"

import { AddComponentSheet } from "./AddComponentSheet"
import {
  ComponentTraySheet,
  type TrayCommitItem,
  type TraySlotContext,
} from "./ComponentTraySheet"
import { DayHeader } from "./DayHeader"
import { SlotCell } from "./SlotCell"
import { SlotSheet, type SlotSheetTarget } from "./SlotSheet"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

export interface PlannerDay {
  date: string // "YYYY-MM-DD"
  weekday: number // 0=Monday…6=Sunday (matches backend convention)
  plates: Plate[]
}

const DAY_KEYS = [
  "planner.day_mon",
  "planner.day_tue",
  "planner.day_wed",
  "planner.day_thu",
  "planner.day_fri",
  "planner.day_sat",
  "planner.day_sun",
] as const

interface PlannerGridProps {
  days: PlannerDay[]
  slots: TimeSlot[]
  rangeFrom: string
  rangeTo: string
  nutritionDays?: NutritionDay[]
}

interface AddTarget {
  day: number
  slotId: number
  plateId: number | null
  defaultRole?: string
}

interface SwapTarget {
  plateId: number
  pcId: number
  defaultRole?: string
}

function SlotIcon({ name }: { name: string }) {
  const Icon = SLOT_ICONS[name] ?? SLOT_ICON_FALLBACK
  return <Icon className="h-4 w-4" aria-hidden />
}

function findPlateInDay(day: PlannerDay, slotId: number): Plate | undefined {
  return day.plates.find((p) => p.slot_id === slotId)
}

function buildTrayContext(
  target: AddTarget,
  days: PlannerDay[],
  slotsById: Map<number, TimeSlot>
): TraySlotContext | null {
  const day = days[target.day]
  const slot = slotsById.get(target.slotId)
  if (!day || !slot) return null
  return {
    slotId: slot.id,
    slotNameKey: slot.name_key,
    date: day.date,
    weekday: day.weekday,
  }
}

export function PlannerGrid({
  days,
  slots,
  rangeFrom,
  rangeTo,
  nutritionDays,
}: PlannerGridProps) {
  const { t } = useTranslation()

  const openPicker = (day: number, slotId: number) => {
    setAddTarget({ day, slotId, plateId: null })
  }

  // Planner needs the full foods catalog to render plate components by id.
  // Treat it as session-cached: a long staleTime stops focus/mount refetches
  // of a multi-MB payload while still letting mutations invalidate it.
  const componentsQuery = useFoods({ limit: 10000 }, { staleTime: 5 * 60_000 })
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

  // Frontend-derived "Recent" tab content for the tray sheet. Walks the
  // plates currently in cache (the visible window), de-duplicates by food id
  // and orders most-recent-first by plate date. Capped at 20 to keep the
  // tab focused. Cheap because it only iterates visible plates — the heavy
  // foods catalog already lives in `componentsById`.
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

  const today = startOfDay(new Date())

  const [addTarget, setAddTarget] = useState<AddTarget | null>(null)
  const [swapTarget, setSwapTarget] = useState<SwapTarget | null>(null)
  const [presetTarget, setPresetTarget] = useState<SaveAsPresetTarget | null>(
    null
  )
  const [sheetTarget, setSheetTarget] = useState<SlotSheetTarget | null>(null)

  function findPlateById(plateId: number): Plate | undefined {
    for (const day of days) {
      const p = day.plates.find((p) => p.id === plateId)
      if (p) return p
    }
    return undefined
  }

  function openSavePreset(plateId: number) {
    const p = findPlateById(plateId)
    if (!p) return
    // Suggest the first component's food name; falls back to "Plate".
    const firstFoodID = p.components[0]?.food_id
    const food =
      firstFoodID != null ? componentsById.get(firstFoodID) : undefined
    setPresetTarget({
      plateIds: [plateId],
      defaultName: food?.name ?? "",
    })
  }

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

  const updatePlateMut = useUpdatePlate(rangeFrom, rangeTo)
  const swapMut = useSwapPlateComponent()
  const setSkippedMut = useSetPlateSkipped()
  const setFavoriteMut = useSetFoodFavorite()
  const recordFeedbackMut = useRecordFeedback()
  const clearFeedbackMut = useClearFeedback()

  // Build the set of "date|slotId" keys for already-occupied slots in the
  // visible window — used by the apply picker to flag overlap and surface
  // The macro target indicator on each cell compares plate kcal to a fair
  // share of the daily target — daily kcal ÷ active slot count. Skipping
  // the calculation whenever no slots or no target is set keeps the dot
  // hidden rather than rendering a misleading 0-target state.
  const { data: profile } = useProfile()
  const kcalPerSlotTarget = useMemo(() => {
    if (!profile?.kcal_target || slots.length === 0) return null
    return profile.kcal_target / slots.length
  }, [profile?.kcal_target, slots.length])

  // Per-plate macros for the visible window. Indexed by plate id so SlotCell
  // can render the kcal pill + P/F/C dots without each cell mounting its own
  // query. The hook key sits under plateKeys.all so any of the existing
  // plate-list invalidations also drops these.
  const { data: plateMacrosData } = usePlateMacros(rangeFrom, rangeTo)
  const macrosByPlateId = useMemo(() => {
    const map = new Map<number, MacrosResponse>()
    for (const entry of plateMacrosData?.plates ?? []) {
      map.set(entry.plate_id, entry.macros)
    }
    return map
  }, [plateMacrosData])

  const aiFill = usePlannerUI((s) => s.aiFill)
  const clearAiFillOnPlate = usePlannerUI((s) => s.clearAiFillOnPlate)
  const markCopyHintSeen = usePlannerUI((s) => s.markCopyHintSeen)
  const aiFilledIds = useMemo(() => new Set(aiFill.plateIds), [aiFill.plateIds])

  async function handleTrayCommit(
    items: TrayCommitItem[]
  ): Promise<{ failedFoodIds: number[] }> {
    if (!addTarget || items.length === 0) return { failedFoodIds: [] }
    const target = addTarget
    const targetDay = days[target.day]
    if (!targetDay) return { failedFoodIds: [] }

    // Commit any plates the user just deleted (within their undo window) so
    // the upcoming refetch doesn't resurrect them as artifacts.
    await flushPendingPlateDeletes()

    let plateId = target.plateId
    if (plateId === null) {
      // If creating the plate fails, nothing landed — every staged item
      // counts as failed so the tray keeps them all for retry.
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
    }

    // Add components in parallel and survive partial failure: report what
    // didn't land so the tray sheet can keep those staged for retry.
    // Pass the kind-aware payload through unchanged. The TrayCommitItem
    // discriminated union exactly matches AddPlateComponentInput, so the
    // composed/leaf split is preserved end-to-end with no translation.
    const results = await Promise.allSettled(
      items.map((it) => addPlateComponent(plateId!, it))
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
    void queryClient.invalidateQueries({ queryKey: shoppingKeys.all })
    void queryClient.invalidateQueries({ queryKey: ["nutrition"] })

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

  async function handleSwapPick(component: Food) {
    if (!swapTarget) return
    const target = swapTarget
    setSwapTarget(null)
    await swapMut.mutateAsync({
      plateId: target.plateId,
      pcId: target.pcId,
      input: { food_id: component.id },
    })
  }

  function handleDeletePlate(plateId: number, dayIdx: number) {
    if (hasPendingPlateDelete(plateId)) return

    const plateSnapshot = days[dayIdx]?.plates.find((p) => p.id === plateId)
    if (!plateSnapshot) return

    // Optimistic: remove from cache immediately so the UI updates at once.
    queryClient.setQueryData<{ plates: Plate[] }>(
      plateKeys.range(rangeFrom, rangeTo),
      (old) => ({ plates: (old?.plates ?? []).filter((p) => p.id !== plateId) })
    )

    registerPendingPlateDelete(plateId, plateSnapshot, async () => {
      try {
        await deletePlate(plateId)
      } catch (err) {
        toastError(err, t)
        queryClient.setQueryData<{ plates: Plate[] }>(
          plateKeys.range(rangeFrom, rangeTo),
          (old) => ({ plates: [...(old?.plates ?? []), plateSnapshot] })
        )
      } finally {
        void queryClient.invalidateQueries({
          queryKey: plateKeys.range(rangeFrom, rangeTo),
        })
        void queryClient.invalidateQueries({ queryKey: ["nutrition"] })
      }
    })

    toast(t("plate.deleted"), {
      action: {
        label: t("common.undo"),
        onClick: () => {
          const snapshot = cancelPendingPlateDelete(plateId)
          if (!snapshot) return
          queryClient.setQueryData<{ plates: Plate[] }>(
            plateKeys.range(rangeFrom, rangeTo),
            (old) => ({ plates: [...(old?.plates ?? []), snapshot] })
          )
        },
      },
      duration: 5000,
    })
  }

  function handleClearDay(dayIdx: number) {
    const targetDay = days[dayIdx]
    if (!targetDay || targetDay.plates.length === 0) return
    const dayPlates = targetDay.plates

    // Optimistic: remove day's plates from cache immediately.
    const dayPlateIds = new Set(dayPlates.map((p) => p.id))
    queryClient.setQueryData<{ plates: Plate[] }>(
      plateKeys.range(rangeFrom, rangeTo),
      (old) => ({
        plates: (old?.plates ?? []).filter((p) => !dayPlateIds.has(p.id)),
      })
    )

    const timeoutId = setTimeout(async () => {
      try {
        await flushPendingPlateDeletes()
        await Promise.all(dayPlates.map((p) => deletePlate(p.id)))
      } catch (err) {
        toastError(err, t)
      } finally {
        void queryClient.invalidateQueries({
          queryKey: plateKeys.range(rangeFrom, rangeTo),
        })
        void queryClient.invalidateQueries({ queryKey: ["nutrition"] })
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

  function handleClearRow(slotId: number) {
    // Skip markers are deliberate user statements ("not eating here") and
    // shouldn't be wiped by "Clear row" — same rule as Copy last week.
    const rowPlates: Plate[] = []
    for (const day of days) {
      const p = findPlateInDay(day, slotId)
      if (p && !p.skipped) rowPlates.push(p)
    }
    if (rowPlates.length === 0) return

    const idSet = new Set(rowPlates.map((p) => p.id))
    queryClient.setQueryData<{ plates: Plate[] }>(
      plateKeys.range(rangeFrom, rangeTo),
      (old) => ({
        plates: (old?.plates ?? []).filter((p) => !idSet.has(p.id)),
      })
    )

    const timeoutId = setTimeout(async () => {
      try {
        await flushPendingPlateDeletes()
        await Promise.all(rowPlates.map((p) => deletePlate(p.id)))
      } catch (err) {
        toastError(err, t)
      } finally {
        void queryClient.invalidateQueries({
          queryKey: plateKeys.range(rangeFrom, rangeTo),
        })
        void queryClient.invalidateQueries({ queryKey: ["nutrition"] })
      }
    }, 5000)

    toast(t("planner.row_actions.cleared", { count: rowPlates.length }), {
      action: {
        label: t("common.undo"),
        onClick: () => {
          clearTimeout(timeoutId)
          queryClient.setQueryData<{ plates: Plate[] }>(
            plateKeys.range(rangeFrom, rangeTo),
            (old) => ({ plates: [...(old?.plates ?? []), ...rowPlates] })
          )
        },
      },
      duration: 5000,
    })
  }

  async function handleCopyRowAcrossWeek(slotId: number) {
    // First non-skipped plate with components becomes the source. Skipped
    // markers and component-less plates would copy nothing useful.
    let source: { plate: Plate; dayIdx: number } | null = null
    for (let i = 0; i < days.length; i++) {
      const day = days[i]
      if (!day) continue
      const p = findPlateInDay(day, slotId)
      if (p && !p.skipped && p.components.length > 0) {
        source = { plate: p, dayIdx: i }
        break
      }
    }
    if (!source) {
      toast(t("planner.row_actions.copy_no_source"))
      return
    }

    // Targets: every other day in the row that's currently empty (no plate
    // and no skip marker). Existing plates and skips are preserved — undo
    // would otherwise need to restore them, and silent overwrite breaks trust.
    const targets = days.filter(
      (d, i) => i !== source!.dayIdx && !findPlateInDay(d, slotId)
    )
    if (targets.length === 0) {
      toast(t("planner.row_actions.copy_no_targets"))
      return
    }

    await flushPendingPlateDeletes()

    try {
      const created = await Promise.all(
        targets.map((d) =>
          createPlate({
            date: d.date,
            slot_id: slotId,
            note: source!.plate.note ?? undefined,
          })
        )
      )
      await Promise.all(
        created.flatMap((p) =>
          source!.plate.components.map((pc) =>
            addPlateComponent(p.id, componentToAddInput(pc))
          )
        )
      )
      void queryClient.invalidateQueries({
        queryKey: plateKeys.range(rangeFrom, rangeTo),
      })
      void queryClient.invalidateQueries({ queryKey: ["nutrition"] })
      toast.success(t("planner.row_actions.copied", { count: targets.length }))
    } catch (err) {
      toastError(err, t)
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
    await toggleSkip({
      date: targetDay.date,
      slotId,
      existing: findPlateInDay(targetDay, slotId),
      noteOverride,
      rangeFrom,
      rangeTo,
      setSkipped: setSkippedMut.mutateAsync,
    })
  }

  async function handleToggleFavorite(
    componentId: number | undefined,
    current: boolean
  ) {
    if (!componentId) return
    await setFavoriteMut.mutateAsync({ id: componentId, favorite: !current })
  }

  async function handleRate(
    plateId: number,
    status: "loved" | "disliked",
    current?: string
  ) {
    if (current === status) {
      await clearFeedbackMut.mutateAsync(plateId)
    } else {
      await recordFeedbackMut.mutateAsync({ plateId, input: { status } })
    }
  }

  // Drag-and-drop: a plain drag is "move", holding ⌘ (macOS) or Ctrl (others)
  // during drag-start switches to "copy". Because dnd-kit doesn't expose the
  // modifier key on drag events, we stash it from the activator event into a
  // ref that onDragEnd reads.
  const headerScrollRef = useRef<HTMLDivElement>(null)
  const bodyScrollRef = useRef<HTMLDivElement>(null)
  const modeRef = useRef<"move" | "copy">("move")
  // Pre-apply snapshot the picker hands us via onBeforeApply, consumed by
  // Drag activates from the dedicated grip handle on the cell's left edge.
  // The 6 px PointerSensor constraint keeps a quick grip-tap from being read
  // as a drag. KeyboardSensor lives on the same handle, so Enter on the
  // stretched-link button reliably opens the sheet — it no longer has the
  // dual-purpose conflict that older revisions worked around.
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor)
  )

  function handleDragStart(event: DragStartEvent) {
    const activator = event.activatorEvent as
      | MouseEvent
      | KeyboardEvent
      | undefined
    const copyHeld =
      !!activator &&
      "metaKey" in activator &&
      (activator.metaKey || activator.ctrlKey)
    modeRef.current = copyHeld ? "copy" : "move"
    const payload: DragPayload | null = dragStartToPayload(event, !!copyHeld)
    if (!payload) modeRef.current = "move"
  }

  async function handleDragEnd(event: DragEndEvent) {
    const active = event.active
    const over = event.over
    if (!over) return
    const activeData = active.data.current as
      | { plateId?: number; day?: number; slotId?: number; date?: string }
      | undefined
    const overData = over.data.current as
      | {
          day?: number
          slotId?: number
          date?: string
          existingPlateId?: number
          skipped?: boolean
        }
      | undefined
    if (!activeData?.plateId || !overData) return
    if (overData.skipped) {
      toast.error(t("planner.dnd.reject_skipped"))
      return
    }
    if (
      activeData.day === overData.day &&
      activeData.slotId === overData.slotId
    ) {
      return
    }

    try {
      if (modeRef.current === "move") {
        await updatePlateMut.mutateAsync({
          id: activeData.plateId,
          input: {
            date: overData.date,
            slot_id: overData.slotId,
          },
        })
      } else {
        const srcDayIdx = activeData.day ?? 0
        const srcDay = days[srcDayIdx]
        const src = srcDay
          ? findPlateInDay(srcDay, activeData.slotId!)
          : undefined
        if (!src) return
        await flushPendingPlateDeletes()
        const created = await createPlate({
          date: overData.date!,
          slot_id: overData.slotId!,
          note: src.note ?? undefined,
        })
        for (const pc of src.components) {
          await addPlateComponent(created.id, componentToAddInput(pc))
        }
        void queryClient.invalidateQueries({
          queryKey: plateKeys.range(rangeFrom, rangeTo),
        })
        void queryClient.invalidateQueries({ queryKey: ["nutrition"] })
        markCopyHintSeen()
      }
    } catch (err) {
      toastError(err, t)
    }
  }

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCorners}
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
    >
      <div>
        {/* Sticky header row — lives outside overflow-x-auto so page-scroll sticky works */}
        <div className="sticky top-16 z-20">
          <div ref={headerScrollRef} className="hide-scrollbar overflow-x-auto">
            <div
              className="grid min-w-[960px] gap-2.5 rounded-t-3xl border-t border-r border-l border-outline-variant/40 bg-surface-container-lowest px-5 pt-5"
              style={{ gridTemplateColumns: "130px repeat(7, minmax(0, 1fr))" }}
            >
              <div />
              {days.map((day, idx) => {
                const date = parseISO(day.date)
                const dayIsToday = isToday(date)
                const dayKey = DAY_KEYS[day.weekday] ?? DAY_KEYS[idx % 7]
                const dayMacros = nutritionDays?.find(
                  (n) => n.date === day.date
                )?.macros
                return (
                  <DayHeader
                    key={day.date}
                    idx={idx}
                    dayKey={dayKey}
                    date={date}
                    today={dayIsToday}
                    macros={dayMacros}
                    onClearDay={() => handleClearDay(idx)}
                    hasPlates={day.plates.length > 0}
                  />
                )
              })}
            </div>
          </div>
        </div>

        {/* Grid body — horizontal scroll synced with header */}
        <div
          ref={bodyScrollRef}
          className="hide-scrollbar overflow-x-auto"
          onScroll={(e) => {
            if (headerScrollRef.current)
              headerScrollRef.current.scrollLeft = e.currentTarget.scrollLeft
          }}
        >
          <div className="editorial-shadow min-w-[960px] rounded-b-3xl border-r border-b border-l border-outline-variant/40 bg-surface-container-lowest px-5 pt-2.5 pb-5">
            <div
              className="grid gap-2.5"
              style={{ gridTemplateColumns: "130px repeat(7, minmax(0, 1fr))" }}
              onKeyDown={(e) =>
                handleGridArrowKey(e, { rows: slots.length, cols: 7 })
              }
            >
              {slots.map((slot, rowIndex) => (
                <div key={slot.id} className="contents">
                  <SlotRowLabel
                    slot={slot}
                    days={days}
                    onClearRow={() => handleClearRow(slot.id)}
                    onCopyAcrossWeek={() => handleCopyRowAcrossWeek(slot.id)}
                  />
                  {days.map((day, dayIdx) => {
                    const date = parseISO(day.date)
                    const isPast = isBefore(date, today) && !isToday(date)
                    const plate = findPlateInDay(day, slot.id)
                    return (
                      <div
                        key={`${slot.id}-${day.date}`}
                        data-today={isToday(date) ? "true" : undefined}
                        data-past={isPast ? "true" : undefined}
                        className={isPast ? "opacity-60" : undefined}
                      >
                        <DndCellWrapper
                          day={dayIdx}
                          date={day.date}
                          slotId={slot.id}
                          rowIndex={rowIndex}
                          plate={plate}
                        >
                          <SlotCell
                            day={dayIdx}
                            slotId={slot.id}
                            date={day.date}
                            slotName={t(slot.name_key, {
                              defaultValue: slot.name_key,
                            })}
                            plate={plate}
                            componentsById={componentsById}
                            macros={
                              plate ? macrosByPlateId.get(plate.id) : undefined
                            }
                            kcalTarget={kcalPerSlotTarget}
                            showMacros={false}
                            aiFilled={plate ? aiFilledIds.has(plate.id) : false}
                            onAdd={() => openPicker(dayIdx, slot.id)}
                            onOpenSheet={
                              plate
                                ? () => openSheetForPlate(dayIdx, slot, plate)
                                : undefined
                            }
                            onDeletePlate={() =>
                              plate && handleDeletePlate(plate.id, dayIdx)
                            }
                            onSaveAsPreset={
                              plate ? () => openSavePreset(plate.id) : undefined
                            }
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
                                dayIdx,
                                slot.id,
                                plate?.id ?? null,
                                note
                              )
                              if (plate) clearAiFillOnPlate(plate.id)
                            }}
                            onRateLoved={() => {
                              if (!plate) return
                              void handleRate(
                                plate.id,
                                "loved",
                                plate.feedback?.status
                              )
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
                        </DndCellWrapper>
                      </div>
                    )
                  })}
                </div>
              ))}
            </div>
          </div>
        </div>

        <ComponentTraySheet
          open={addTarget !== null}
          context={
            addTarget ? buildTrayContext(addTarget, days, slotsById) : null
          }
          recentFoods={recentFoods}
          existingComponents={
            addTarget
              ? (findPlateInDay(days[addTarget.day]!, addTarget.slotId)
                  ?.components ?? [])
              : []
          }
          foodById={componentsById}
          onOpenChange={(o) => !o && setAddTarget(null)}
          onCommit={(items) => handleTrayCommit(items)}
        />
        <AddComponentSheet
          open={swapTarget !== null}
          onOpenChange={(o) => !o && setSwapTarget(null)}
          defaultRole={swapTarget?.defaultRole}
          onPick={handleSwapPick}
        />
        <SaveAsPresetDialog
          open={presetTarget !== null}
          onOpenChange={(o) => !o && setPresetTarget(null)}
          target={presetTarget}
        />
        <SlotSheet
          target={sheetTarget}
          days={days}
          componentsById={componentsById}
          rangeFrom={rangeFrom}
          rangeTo={rangeTo}
          aiFilled={sheetTarget ? aiFilledIds.has(sheetTarget.plateId) : false}
          onOpenChange={(open) => {
            if (!open) setSheetTarget(null)
          }}
          onAddComponent={(target) =>
            setAddTarget({
              day: days.findIndex((d) => d.date === target.date),
              slotId: target.slotId,
              plateId: target.plateId,
            })
          }
          onSwapComponent={(target, pcId, defaultRole) =>
            setSwapTarget({
              plateId: target.plateId,
              pcId,
              defaultRole,
            })
          }
          onSaveAsPreset={(plateId) => openSavePreset(plateId)}
        />
      </div>
    </DndContext>
  )
}

interface SlotRowLabelProps {
  slot: TimeSlot
  days: PlannerDay[]
  onClearRow: () => void
  onCopyAcrossWeek: () => void
}

function SlotRowLabel({
  slot,
  days,
  onClearRow,
  onCopyAcrossWeek,
}: SlotRowLabelProps) {
  const { t } = useTranslation()
  // Counted once per row render — drives whether destructive items render at
  // all. Skipped plates aren't counted: "Clear row" preserves them, so the
  // label and the action stay aligned. A row with no plates can still be the
  // target of "apply template", so the menu itself stays available.
  let plateCount = 0
  for (const day of days) {
    const p = findPlateInDay(day, slot.id)
    if (p && !p.skipped) plateCount++
  }
  return (
    <div
      className="group/row relative flex flex-col items-center justify-center gap-1.5 px-3"
      data-testid={`slot-row-${slot.id}`}
    >
      <span className="grid size-6 place-items-center rounded-lg bg-surface-container text-on-surface-variant">
        <SlotIcon name={slot.icon} />
      </span>
      <span className="font-heading text-[12.5px] font-bold tracking-[0.04em] text-on-surface uppercase">
        {slotLabel(t, slot.name_key)}
      </span>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t("planner.row_actions.menu_label", {
              slot: slotLabel(t, slot.name_key),
            })}
            data-testid={`slot-row-menu-${slot.id}`}
            className="absolute top-1 right-1 size-5 text-on-surface-variant/60 opacity-0 transition-opacity group-hover/row:opacity-100 hover:text-on-surface-variant focus-visible:opacity-100 data-[state=open]:opacity-100"
          >
            <MoreHorizontal className="h-3 w-3" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-60">
          <DropdownMenuItem
            onClick={onCopyAcrossWeek}
            disabled={plateCount === 0}
            data-testid={`slot-row-copy-${slot.id}`}
          >
            <Copy className="size-4" />
            {t("planner.row_actions.copy_across")}
          </DropdownMenuItem>
          {plateCount > 0 && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                onClick={onClearRow}
                variant="destructive"
                data-testid={`slot-row-clear-${slot.id}`}
              >
                <Trash2 className="size-4" />
                {t("planner.row_actions.clear", { count: plateCount })}
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
